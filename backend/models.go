package main

// This file is the single catalogue of values that cross a HTTP/SQLite
// boundary. Keeping request, response and persistence shapes together makes
// the JSON contract visible without having to search through handlers.

// apiError is returned for every failed JSON request.
type apiError struct {
	Error string `json:"error"`
}

// healthResponse is returned by the liveness/readiness endpoint.
type healthResponse struct {
	OK       bool   `json:"ok"`
	Database bool   `json:"database"`
	Version  string `json:"version"`
	Commit   string `json:"commit"`
}

// authResponse is shared by setup, login, logout and auth status.
type authResponse struct {
	Initialized   bool   `json:"initialized"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
}

// dashboardResponse contains the four counters shown in the teacher home.
type dashboardResponse struct {
	ClassCount     int `json:"classCount"`
	StudentCount   int `json:"studentCount"`
	TotalScore     int `json:"totalScore"`
	ActiveSessions int `json:"activeSessions"`
}

// operationResponse is the common result for successful delete/update calls.
type operationResponse struct {
	OK bool `json:"ok"`
}

// publicStudent is the intentionally small student shape used by self check-in.
type publicStudent struct {
	ID        int64  `json:"id"`
	StudentNo string `json:"studentNo"`
	Name      string `json:"name"`
	CheckedIn bool   `json:"checkedIn"`
}

// publicRoster is the active session and its selectable students.
type publicRoster struct {
	SessionID int64           `json:"sessionId"`
	Title     string          `json:"title"`
	Course    string          `json:"course"`
	SessionAt string          `json:"sessionAt"`
	Students  []publicStudent `json:"students"`
}

// checkInResponse confirms a successful self check-in.
type checkInResponse struct {
	OK      bool   `json:"ok"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

// backupRestoreResponse identifies the safety copy created before a restore.
type backupRestoreResponse struct {
	OK           bool   `json:"ok"`
	SafetyBackup string `json:"safetyBackup"`
	Message      string `json:"message"`
}

// qrLoginStartResponse contains the two one-time tokens used by QR login.
type qrLoginStartResponse struct {
	Token      string `json:"token"`
	ClaimToken string `json:"claimToken"`
	ExpiresAt  int64  `json:"expiresAt"`
}

// qrAuthenticatedResponse is returned when the phone successfully claims a QR login.
type qrAuthenticatedResponse struct {
	Status        string `json:"status"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
}

// teacherCredentials is used by setup, login and deployment bootstrap.
type teacherCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// classInput is the write model for a class. Name is derived from grade and
// classNo by validation and is kept for backwards-compatible decoding.
type classInput struct {
	Name    string `json:"name"`
	Grade   string `json:"grade"`
	ClassNo string `json:"classNo"`
}

// studentInput is the write model for a student.
type studentInput struct {
	StudentNo string `json:"studentNo"`
	Name      string `json:"name"`
}

// scoreInput describes one auditable score adjustment.
type scoreInput struct {
	Delta  int    `json:"delta"`
	Reason string `json:"reason"`
}

// attendanceInput starts a new attendance session.
type attendanceInput struct {
	ClassID   int64  `json:"classId"`
	Title     string `json:"title"`
	Course    string `json:"course"`
	SessionAt string `json:"sessionAt"`
}

// scheduleInput is the write model for a regular timetable lesson.
type scheduleInput struct {
	ClassID      int64  `json:"classId"`
	Course       string `json:"course"`
	Weekday      int    `json:"weekday"`
	Period       int    `json:"period"`
	LocationOdd  string `json:"locationOdd"`
	LocationEven string `json:"locationEven"`
}

// scheduleSettingsInput updates the semester range and seven periods.
type scheduleSettingsInput struct {
	SemesterStart string           `json:"semesterStart"`
	SemesterEnd   string           `json:"semesterEnd"`
	Periods       []schedulePeriod `json:"periods"`
}

// scheduleChangeInput describes an occupied or rescheduled lesson date.
type scheduleChangeInput struct {
	Date         string `json:"date"`
	Status       string `json:"status"`
	NewDate      string `json:"newDate"`
	NewStartTime string `json:"newStartTime"`
	NewEndTime   string `json:"newEndTime"`
	NewClassID   int64  `json:"newClassId"`
	Note         string `json:"note"`
}

// siteSettings contains the two public branding strings.
type siteSettings struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
}

// teacherAccount is the database representation of the sole teacher account.
type teacherAccount struct {
	Username     string
	PasswordHash string
}

// navigationLinkInput is the write model for an external or uploaded site.
type navigationLinkInput struct {
	ID      int64  `json:"id,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	IconURL string `json:"iconUrl"`
}

// navigationBatchInput replaces the complete ordered navigation list.
type navigationBatchInput struct {
	Items []navigationLinkInput `json:"items"`
}

// teachingSiteSummary describes an uploaded static teaching site revision.
type teachingSiteSummary struct {
	PublicID      string `json:"publicId"`
	Revision      string `json:"revision"`
	SourceName    string `json:"sourceName"`
	SourceSize    int64  `json:"sourceSize"`
	ExtractedSize int64  `json:"extractedSize"`
	FileCount     int    `json:"fileCount"`
	UpdatedAt     string `json:"updatedAt"`
}

// navigationLink is the public/admin representation of a navigation item.
type navigationLink struct {
	ID        int64                `json:"id"`
	Kind      string               `json:"kind"`
	Title     string               `json:"title"`
	URL       string               `json:"url"`
	IconURL   string               `json:"iconUrl"`
	SortOrder int                  `json:"sortOrder"`
	Site      *teachingSiteSummary `json:"site,omitempty"`
}

// classRow is the shared class response used by the teacher and public APIs.
type classRow struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Grade           string `json:"grade"`
	ClassNo         string `json:"classNo"`
	StudentCount    int    `json:"studentCount"`
	TotalScore      int    `json:"totalScore"`
	ActiveSessionID *int64 `json:"activeSessionId"`
	CreatedAt       string `json:"createdAt"`
}

// studentRow is the teacher-facing student response.
type studentRow struct {
	ID        int64  `json:"id"`
	ClassID   int64  `json:"classId"`
	StudentNo string `json:"studentNo"`
	Name      string `json:"name"`
	Score     int    `json:"score"`
	CreatedAt string `json:"createdAt"`
}

// scoreEvent is an immutable score journal entry. Reversals append a new
// event instead of deleting the original entry.
type scoreEvent struct {
	ID         int64   `json:"id"`
	Delta      int     `json:"delta"`
	Reason     string  `json:"reason"`
	ReversalOf *int64  `json:"reversalOf"`
	ReversedAt *string `json:"reversedAt"`
	Reversible bool    `json:"reversible"`
	CreatedAt  string  `json:"createdAt"`
}

// attendanceRecord stores the student snapshot used by historical sessions.
type attendanceRecord struct {
	StudentID int64   `json:"studentId"`
	StudentNo string  `json:"studentNo"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	CheckedAt *string `json:"checkedAt"`
	Method    string  `json:"method"`
}

// attendanceView is the detailed attendance response.
type attendanceView struct {
	ID           int64              `json:"id"`
	ClassID      int64              `json:"classId"`
	ClassName    string             `json:"className"`
	Title        string             `json:"title"`
	Course       string             `json:"course"`
	Status       string             `json:"status"`
	StartedAt    string             `json:"startedAt"`
	SessionAt    string             `json:"sessionAt"`
	EndedAt      *string            `json:"endedAt"`
	DeletedAt    *string            `json:"deletedAt"`
	PresentCount int                `json:"presentCount"`
	AbsentCount  int                `json:"absentCount"`
	Records      []attendanceRecord `json:"records"`
}

// attendancePage is the cursor-paginated attendance list response.
type attendancePage struct {
	Items      []attendanceView `json:"items"`
	NextCursor int64            `json:"nextCursor"`
}

// attendanceSuggestion is the server-side timetable recommendation.
type attendanceSuggestion struct {
	Detected   bool   `json:"detected"`
	ServerTime string `json:"serverTime"`
	ClassID    int64  `json:"classId"`
	ClassName  string `json:"className"`
	Course     string `json:"course"`
	SessionAt  string `json:"sessionAt"`
	Period     int    `json:"period"`
	StartTime  string `json:"startTime"`
	EndTime    string `json:"endTime"`
	Source     string `json:"source"`
	Message    string `json:"message"`
}

// scheduleLesson is one regular timetable entry.
type scheduleLesson struct {
	ID           int64  `json:"id"`
	ClassID      int64  `json:"classId"`
	ClassName    string `json:"className"`
	Course       string `json:"course"`
	Weekday      int    `json:"weekday"`
	Period       int    `json:"period"`
	StartTime    string `json:"startTime"`
	EndTime      string `json:"endTime"`
	LocationOdd  string `json:"locationOdd"`
	LocationEven string `json:"locationEven"`
}

// schedulePeriod is one of the seven configured class periods.
type schedulePeriod struct {
	Period    int    `json:"period"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

// scheduleSettings is the complete timetable configuration.
type scheduleSettings struct {
	SemesterStart string           `json:"semesterStart"`
	SemesterEnd   string           `json:"semesterEnd"`
	Periods       []schedulePeriod `json:"periods"`
}

// scheduleChange is a date-specific timetable override.
type scheduleChange struct {
	ID           int64  `json:"id"`
	LessonID     int64  `json:"lessonId"`
	Date         string `json:"date"`
	Status       string `json:"status"`
	NewDate      string `json:"newDate"`
	NewStartTime string `json:"newStartTime"`
	NewEndTime   string `json:"newEndTime"`
	NewClassID   int64  `json:"newClassId"`
	NewClassName string `json:"newClassName"`
	Note         string `json:"note"`
}

// scheduleData is returned by GET /api/schedule.
type scheduleData struct {
	Lessons  []scheduleLesson `json:"lessons"`
	Changes  []scheduleChange `json:"changes"`
	Settings scheduleSettings `json:"settings"`
}

// auditLog is the append-only operations history entry.
type auditLog struct {
	ID         int64  `json:"id"`
	Action     string `json:"action"`
	EntityType string `json:"entityType"`
	EntityID   *int64 `json:"entityId"`
	Summary    string `json:"summary"`
	Details    string `json:"details"`
	Actor      string `json:"actor"`
	CreatedAt  string `json:"createdAt"`
}

// qrLoginRecord is the state polled by the computer and phone during QR login.
type qrLoginRecord struct {
	DeviceName string `json:"deviceName"`
	Status     string `json:"status"`
	ExpiresAt  int64  `json:"expiresAt"`
}

// qrLoginTokenInput carries the short-lived claim token from the phone.
type qrLoginTokenInput struct {
	Token string `json:"token"`
}

// scheduleImportRow is the normalized intermediate representation of an Excel
// timetable row before class IDs and period numbers are resolved.
type scheduleImportRow struct {
	ClassName    string
	Course       string
	Weekday      int
	StartTime    string
	EndTime      string
	LocationOdd  string
	LocationEven string
}

// integrationClassesResponse is intentionally smaller than classRow so the
// integration API never exposes scores, attendance or internal timestamps.
type integrationClassesResponse struct {
	Classes []integrationClass `json:"classes"`
}

type integrationClass struct {
	ID       string               `json:"id"`
	Name     string               `json:"name"`
	Students []integrationStudent `json:"students"`
}

type integrationStudent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
