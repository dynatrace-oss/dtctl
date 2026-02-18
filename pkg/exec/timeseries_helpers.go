package exec

import (
	"fmt"
	"time"
)

// TimeseriesPoint represents a numeric datapoint with its timestamp.
type TimeseriesPoint struct {
	Value     float64
	Timestamp time.Time
}

// ExtractQueryRecords normalizes records returned by DQL APIs across legacy and nested response shapes.
func ExtractQueryRecords(result *DQLQueryResponse) []map[string]interface{} {
	if result == nil {
		return nil
	}
	if result.Result != nil && len(result.Result.Records) > 0 {
		return result.Result.Records
	}
	return result.Records
}

// ExtractLatestPointFromTimeseries returns the latest non-null numeric point for a timeseries field.
func ExtractLatestPointFromTimeseries(records []map[string]interface{}, field string) (TimeseriesPoint, bool) {
	var latest TimeseriesPoint
	found := false

	for _, rec := range records {
		values, ok := rec[field].([]interface{})
		if !ok || len(values) == 0 {
			continue
		}

		for idx := len(values) - 1; idx >= 0; idx-- {
			if values[idx] == nil {
				continue
			}

			number, ok := toFloat64(values[idx])
			if !ok {
				continue
			}

			point := TimeseriesPoint{Value: number}
			if ts, ok := extractTimestampForIndex(rec, idx, len(values)); ok {
				point.Timestamp = ts
			}

			if !found {
				latest = point
				found = true
				break
			}

			if !point.Timestamp.IsZero() {
				if latest.Timestamp.IsZero() || point.Timestamp.After(latest.Timestamp) {
					latest = point
				}
			}

			break
		}
	}

	return latest, found
}

func toFloat64(value interface{}) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}

func extractTimestampForIndex(record map[string]interface{}, idx int, seriesLen int) (time.Time, bool) {
	tfRaw, ok := record["timeframe"].(map[string]interface{})
	if !ok {
		return time.Time{}, false
	}

	interval, ok := parseRecordInterval(record["interval"])
	if !ok || interval <= 0 {
		return time.Time{}, false
	}

	if start, ok := parseTimeFromMap(tfRaw, "start"); ok {
		return start.Add(time.Duration(idx) * interval), true
	}

	if end, ok := parseTimeFromMap(tfRaw, "end"); ok {
		stepsBack := seriesLen - 1 - idx
		return end.Add(-time.Duration(stepsBack) * interval), true
	}

	return time.Time{}, false
}

func parseRecordInterval(raw interface{}) (time.Duration, bool) {
	switch value := raw.(type) {
	case string:
		var intervalNs int64
		if _, err := fmt.Sscanf(value, "%d", &intervalNs); err == nil && intervalNs > 0 {
			return time.Duration(intervalNs), true
		}
	case float64:
		if value > 0 {
			return time.Duration(int64(value)), true
		}
	case int64:
		if value > 0 {
			return time.Duration(value), true
		}
	case int:
		if value > 0 {
			return time.Duration(value), true
		}
	}

	return time.Duration(0), false
}

func parseTimeFromMap(tf map[string]interface{}, key string) (time.Time, bool) {
	raw, ok := tf[key]
	if !ok {
		return time.Time{}, false
	}

	s, ok := raw.(string)
	if !ok || s == "" {
		return time.Time{}, false
	}

	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04Z",
	}

	for _, format := range formats {
		if parsed, err := time.Parse(format, s); err == nil {
			return parsed, true
		}
	}

	return time.Time{}, false
}
