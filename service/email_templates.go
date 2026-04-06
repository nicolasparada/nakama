package service

import (
	"fmt"
	"html/template"
	"time"
)

var emailTemplateFuncs = template.FuncMap{
	"humanDuration": humanDuration,
}

func humanDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	type unit struct {
		duration time.Duration
		singular string
		plural   string
	}

	units := []unit{
		{duration: 24 * time.Hour, singular: "day", plural: "days"},
		{duration: time.Hour, singular: "hour", plural: "hours"},
		{duration: time.Minute, singular: "minute", plural: "minutes"},
		{duration: time.Second, singular: "second", plural: "seconds"},
	}

	for _, unit := range units {
		if d >= unit.duration {
			value := d / unit.duration
			label := unit.plural
			if value == 1 {
				label = unit.singular
			}

			return fmt.Sprintf("%d %s", value, label)
		}
	}

	return d.String()
}
