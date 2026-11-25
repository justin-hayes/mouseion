package domain

import (
	"time"
	// The Study day is computed in Europe/Berlin, so the zone database must be
	// available on hosts that do not ship one.
	_ "time/tzdata"
)

// StudyDayStartHour is the hour, in Europe/Berlin, at which a Study day
// begins. It is a pilot constant (spec #2691).
const StudyDayStartHour = 5

var studyLocation = mustLoadStudyLocation()

func mustLoadStudyLocation() *time.Location {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic("domain: load Europe/Berlin: " + err.Error())
	}
	return location
}

// StudyDayStart is the instant the Study day containing t began: 05:00 in
// Europe/Berlin on t's Berlin date, or on the day before when t falls before
// 05:00. Computing it from the Berlin calendar date keeps it correct across
// daylight-saving changes.
func StudyDayStart(t time.Time) time.Time {
	local := t.In(studyLocation)
	start := time.Date(local.Year(), local.Month(), local.Day(), StudyDayStartHour, 0, 0, 0, studyLocation)
	if local.Before(start) {
		start = time.Date(local.Year(), local.Month(), local.Day()-1, StudyDayStartHour, 0, 0, 0, studyLocation)
	}
	return start
}

// NextStudyDayStart is the instant the Study day after the one containing t
// begins.
func NextStudyDayStart(t time.Time) time.Time {
	start := StudyDayStart(t)
	return time.Date(start.Year(), start.Month(), start.Day()+1, StudyDayStartHour, 0, 0, 0, studyLocation)
}

// StudyDay names the Study day containing t by the Berlin calendar date it
// began on, as YYYY-MM-DD.
func StudyDay(t time.Time) string {
	return StudyDayStart(t).Format(time.DateOnly)
}
