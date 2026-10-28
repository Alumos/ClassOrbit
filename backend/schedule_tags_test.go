package main

import (
	"testing"
	"time"
)

func TestScheduleChangeTagRoundTrip(t *testing.T) {
	s := testStore(t)
	class, err := s.createClass(classInput{Grade: "三", ClassNo: "1"})
	if err != nil {
		t.Fatal(err)
	}
	lesson, err := s.createScheduleLesson(scheduleInput{ClassID: class.ID, Course: "信息科技", Weekday: 1, Period: 1, LocationOdd: "机房 1", LocationEven: "机房 1"})
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().Format("2006-01-02")
	change, err := s.setScheduleChange(lesson.ID, scheduleChangeInput{Date: date, Status: "occupied", Tag: "conflict", Note: "学校运动会"})
	if err != nil {
		t.Fatal(err)
	}
	if change.Tag != "conflict" {
		t.Fatalf("tag = %q, want conflict", change.Tag)
	}
	data, err := s.schedule()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Changes) != 1 || data.Changes[0].Tag != "conflict" {
		t.Fatalf("schedule changes = %+v", data.Changes)
	}
	if _, err := s.setScheduleChange(lesson.ID, scheduleChangeInput{Date: date, Status: "occupied", Tag: "occupied", Note: "机房被占用"}); err != nil {
		t.Fatal(err)
	}
	data, err = s.schedule()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Changes) != 1 || data.Changes[0].Tag != "occupied" {
		t.Fatalf("upsert did not update the tag: %+v", data.Changes)
	}
}
