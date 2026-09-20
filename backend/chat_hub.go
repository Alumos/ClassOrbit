package main

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"
)

const (
	chatMaxRooms        = 10
	chatMaxConnections  = 500
	chatRoomConnections = 100
	chatMaxMessages     = 500
	chatMaxMessageBytes = 1 << 20
	chatMessageTTL      = 2 * time.Hour
	chatQueueEvents     = 64
	chatQueueBytes      = 256 << 10
	chatSnapshotBytes   = 200 << 10
	chatDedupeLimit     = 2000
	chatDedupeTTL       = 10 * time.Minute
)

type chatFailure struct {
	status        int
	code, message string
}

func (e *chatFailure) Error() string                  { return e.message }
func chatFail(status int, code, message string) error { return &chatFailure{status, code, message} }

type chatEvent struct{ wire []byte }
type chatClient struct {
	roomID      int64
	sender      chatSender
	tokenHash   string
	events      chan chatEvent
	done        chan struct{}
	queuedBytes int
}
type chatReceipt struct {
	message chatMessage
	at      time.Time
}
type chatBucket struct {
	tokens float64
	at     time.Time
}

func (b *chatBucket) allow(now time.Time, rate, burst float64) bool {
	if b.at.IsZero() {
		b.tokens = burst
	} else {
		b.tokens = min(burst, b.tokens+max(0, now.Sub(b.at).Seconds())*rate)
	}
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type chatRoom struct {
	id       int64
	epoch    string
	seq      uint64
	paused   bool
	messages []chatMessage
	bytes    int
	clients  map[*chatClient]bool
	receipts map[string]chatReceipt
	rates    map[string]*chatBucket
	rate     chatBucket
	idleAt   time.Time
}

// The bounded hub uses one short state lock. No network, database or disk I/O
// occurs under it. HTTP authorization is separately serialized by chatAccessMu.
type chatHub struct {
	mu          sync.Mutex
	rooms       map[int64]*chatRoom
	clients     map[*chatClient]bool
	stopped     bool
	controlRate chatBucket
}

func newChatHub() *chatHub {
	return &chatHub{rooms: make(map[int64]*chatRoom), clients: make(map[*chatClient]bool)}
}
func (h *chatHub) roomLocked(id int64, now time.Time) (*chatRoom, error) {
	if h.stopped {
		return nil, chatFail(503, "chat_stopped", "聊天室正在关闭")
	}
	if room := h.rooms[id]; room != nil {
		return room, nil
	}
	if len(h.rooms) >= chatMaxRooms {
		return nil, chatFail(503, "chat_capacity", "聊天室容量已满，请稍后再试")
	}
	epoch, err := randomURLToken()
	if err != nil {
		return nil, err
	}
	room := &chatRoom{id: id, epoch: epoch, clients: make(map[*chatClient]bool), receipts: make(map[string]chatReceipt), rates: make(map[string]*chatBucket), idleAt: now}
	h.rooms[id] = room
	return room, nil
}
func (r *chatRoom) online() int {
	ids := make(map[string]bool)
	for c := range r.clients {
		if c.sender.Role == "student" {
			ids[c.sender.ID] = true
		}
	}
	return len(ids)
}
func (r *chatRoom) snapshot() chatSnapshot {
	start := max(0, len(r.messages)-100)
	messages := append([]chatMessage{}, r.messages[start:]...)
	snapshot := chatSnapshot{r.id, r.epoch, r.seq, messages, r.online(), r.paused}
	// Escaped JSON can be larger than UTF-8 text, including control characters.
	for len(snapshot.Messages) > 0 {
		encoded, _ := json.Marshal(snapshot)
		if len(encoded) <= chatSnapshotBytes {
			break
		}
		snapshot.Messages = snapshot.Messages[1:]
	}
	return snapshot
}
func encodeChatEvent(kind string, value any) chatEvent {
	body, _ := json.Marshal(value)
	return chatEvent{wire: []byte("event: " + kind + "\ndata: " + string(body) + "\n\n")}
}
func (h *chatHub) dropLocked(c *chatClient) {
	if !h.clients[c] {
		return
	}
	delete(h.clients, c)
	if room := h.rooms[c.roomID]; room != nil {
		delete(room.clients, c)
		room.idleAt = time.Now()
	}
	close(c.done)
	// Channels remain open: the writer selects done, and no producer can panic.
	for {
		select {
		case <-c.events:
		default:
			c.queuedBytes = 0
			return
		}
	}
}
func (h *chatHub) enqueueLocked(c *chatClient, event chatEvent) {
	if !h.clients[c] {
		return
	}
	if c.queuedBytes+len(event.wire) > chatQueueBytes {
		h.dropLocked(c)
		return
	}
	select {
	case c.events <- event:
		c.queuedBytes += len(event.wire)
	default:
		h.dropLocked(c)
	}
}
func (h *chatHub) broadcastLocked(room *chatRoom, kind string, value any) {
	event := encodeChatEvent(kind, value)
	for c := range room.clients {
		h.enqueueLocked(c, event)
	}
}
func (h *chatHub) presenceLocked(room *chatRoom) {
	h.broadcastLocked(room, "presence", map[string]any{"epoch": room.epoch, "onlineStudents": room.online()})
}
func (h *chatHub) pruneRoomLocked(r *chatRoom, now time.Time) {
	removed := false
	for len(r.messages) > 0 && (len(r.messages) > chatMaxMessages || r.bytes > chatMaxMessageBytes || !r.messages[0].CreatedAt.Add(chatMessageTTL).After(now)) {
		r.bytes -= len(r.messages[0].Text)
		r.messages[0] = chatMessage{}
		r.messages = r.messages[1:]
		removed = true
	}
	for key, receipt := range r.receipts {
		if !receipt.at.Add(chatDedupeTTL).After(now) {
			delete(r.receipts, key)
		}
	}
	for key, bucket := range r.rates {
		if now.Sub(bucket.at) > time.Minute {
			delete(r.rates, key)
		}
	}
	if removed {
		firstSeq := r.seq + 1
		if len(r.messages) > 0 {
			firstSeq = r.messages[0].Seq
		}
		h.broadcastLocked(r, "history_trimmed", map[string]any{"epoch": r.epoch, "firstSeq": firstSeq})
	}
}
func (h *chatHub) subscribe(id int64, sender chatSender, hash string, now time.Time) (*chatClient, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) >= chatMaxConnections {
		return nil, chatFail(429, "connection_limit", "连接过多，请关闭多余标签页")
	}
	same, teachers := 0, 0
	for c := range h.clients {
		if c.sender.Role == "teacher" {
			teachers++
		}
		if c.sender.Role == sender.Role && c.sender.ID == sender.ID {
			same++
		}
	}
	if (sender.Role == "student" && same >= 2) || (sender.Role == "teacher" && teachers >= 10) {
		return nil, chatFail(429, "connection_limit", "请关闭多余的聊天室标签页")
	}
	room, err := h.roomLocked(id, now)
	if err != nil {
		return nil, err
	}
	if len(room.clients) >= chatRoomConnections {
		return nil, chatFail(429, "connection_limit", "本课堂连接已满")
	}
	h.pruneRoomLocked(room, now)
	c := &chatClient{roomID: id, sender: sender, tokenHash: hash, events: make(chan chatEvent, chatQueueEvents), done: make(chan struct{})}
	h.clients[c] = true
	room.clients[c] = true
	h.enqueueLocked(c, encodeChatEvent("snapshot", room.snapshot()))
	h.presenceLocked(room)
	return c, nil
}
func (h *chatHub) unsubscribe(c *chatClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dropLocked(c)
	if room := h.rooms[c.roomID]; room != nil {
		h.presenceLocked(room)
	}
}
func (h *chatHub) consumed(c *chatClient, event chatEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c.queuedBytes = max(0, c.queuedBytes-len(event.wire))
}
func (h *chatHub) send(id int64, sender chatSender, input chatMessageInput, now time.Time) (chatMessage, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room, err := h.roomLocked(id, now)
	if err != nil {
		return chatMessage{}, false, err
	}
	if input.Epoch != room.epoch {
		return chatMessage{}, false, chatFail(409, "stale_epoch", "消息已清空，请等待重新连接后发送")
	}
	h.pruneRoomLocked(room, now)
	key := sender.Role + ":" + sender.ID + ":" + input.ClientMessageID
	if receipt, ok := room.receipts[key]; ok {
		if receipt.message.Text != input.Text {
			return chatMessage{}, false, chatFail(409, "message_conflict", "重试内容不一致，请重新编辑后发送")
		}
		return receipt.message, true, nil
	}
	if room.paused && sender.Role == "student" {
		return chatMessage{}, false, chatFail(409, "room_paused", "老师已暂停学生发言")
	}
	rateKey := sender.Role + ":" + sender.ID
	if room.rates[rateKey] == nil {
		if len(room.rates) >= 1000 {
			return chatMessage{}, false, chatFail(429, "rate_limit", "请求过于频繁")
		}
		room.rates[rateKey] = &chatBucket{}
	}
	if !room.rates[rateKey].allow(now, 1, 3) || !room.rate.allow(now, 20, 40) {
		return chatMessage{}, false, chatFail(429, "rate_limit", "发送太快，请稍后再试")
	}
	if len(room.receipts) >= chatDedupeLimit {
		oldestKey := ""
		var oldest time.Time
		for k, v := range room.receipts {
			if oldestKey == "" || v.at.Before(oldest) {
				oldestKey = k
				oldest = v.at
			}
		}
		delete(room.receipts, oldestKey)
	}
	room.seq++
	message := chatMessage{ID: room.epoch + ":" + strconv.FormatUint(room.seq, 10), Seq: room.seq, ClientMessageID: input.ClientMessageID, Sender: sender, Text: input.Text, CreatedAt: now.UTC()}
	room.messages = append(room.messages, message)
	room.bytes += len(message.Text)
	room.idleAt = now
	room.receipts[key] = chatReceipt{message, now}
	h.broadcastLocked(room, "message", map[string]any{"epoch": room.epoch, "message": message})
	h.pruneRoomLocked(room, now)
	return message, false, nil
}
func (h *chatHub) stats(id int64) (count, online int, paused bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.rooms[id]; r != nil {
		return len(r.messages), r.online(), r.paused
	}
	return
}
func (h *chatHub) manage(id int64, paused *bool, epoch string, now time.Time) (chatSnapshot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room, err := h.roomLocked(id, now)
	if err != nil {
		return chatSnapshot{}, err
	}
	if paused != nil {
		room.paused = *paused
		h.broadcastLocked(room, "room_state", map[string]any{"epoch": room.epoch, "paused": room.paused})
	} else {
		room.epoch = epoch
		room.seq = 0
		room.messages = nil
		room.bytes = 0
		room.receipts = make(map[string]chatReceipt)
		// Drop queued old events before publishing the new generation.
		for c := range room.clients {
		drain:
			for {
				select {
				case event := <-c.events:
					c.queuedBytes = max(0, c.queuedBytes-len(event.wire))
				default:
					break drain
				}
			}
		}
		h.broadcastLocked(room, "room_cleared", room.snapshot())
	}
	return room.snapshot(), nil
}
func (h *chatHub) credentials() (students, teachers []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st, tc := map[string]bool{}, map[string]bool{}
	for c := range h.clients {
		if c.sender.Role == "teacher" {
			tc[c.tokenHash] = true
		} else {
			st[c.tokenHash] = true
		}
	}
	for hash := range st {
		students = append(students, hash)
	}
	for hash := range tc {
		teachers = append(teachers, hash)
	}
	return
}
func (h *chatHub) reconcile(rooms map[int64]bool, students, teachers map[string]bool, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, r := range h.rooms {
		if !rooms[id] {
			for c := range r.clients {
				h.dropLocked(c)
			}
			delete(h.rooms, id)
			continue
		}
		for c := range r.clients {
			valid := students[c.tokenHash]
			if c.sender.Role == "teacher" {
				valid = teachers[c.tokenHash]
			}
			if !valid {
				h.dropLocked(c)
			}
		}
		h.pruneRoomLocked(r, now)
		h.presenceLocked(r)
		if len(r.clients) == 0 && len(r.messages) == 0 && now.Sub(r.idleAt) > chatMessageTTL {
			delete(h.rooms, id)
		}
	}
}
func (h *chatHub) reset(stop bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		h.dropLocked(c)
	}
	h.rooms = make(map[int64]*chatRoom)
	h.stopped = stop
}
func (h *chatHub) ensure(id int64, now time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.roomLocked(id, now)
	return err
}

func (h *chatHub) allowControl(now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.controlRate.allow(now, 2, 10)
}
