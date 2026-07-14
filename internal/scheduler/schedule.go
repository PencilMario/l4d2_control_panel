package scheduler

import (
	"github.com/robfig/cron/v3"
	"time"
)

type Schedule struct {
	schedule cron.Schedule
	location *time.Location
}

func Parse(expression, timezone string) (Schedule, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return Schedule{}, err
	}
	parsed, err := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(expression)
	if err != nil {
		return Schedule{}, err
	}
	return Schedule{schedule: parsed, location: location}, nil
}
func (s Schedule) Next(after time.Time) time.Time { return s.schedule.Next(after.In(s.location)) }
