package cdb

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

// StatusPeriod is a period during which a service, or an instance, kept the
// same statuses: Avail always, Overall for an instance.
type StatusPeriod struct {
	Avail   string
	Overall string
	Begin   string
	End     string
}

const periodDateFormat = "'%Y-%m-%d %H:%i:%s'"

// ServiceStatusPeriods returns the availability periods of a service ending in
// the last days: the closed ones of services_log, then the current one of
// services_log_last, oldest first.
func (oDb *DB) ServiceStatusPeriods(ctx context.Context, svcID string, days int) ([]StatusPeriod, error) {
	query := `SELECT svc_availstatus, DATE_FORMAT(svc_begin, ` + periodDateFormat + `), DATE_FORMAT(svc_end, ` + periodDateFormat + `)
		FROM (
			SELECT svc_availstatus, svc_begin, svc_end FROM services_log WHERE svc_id = ? AND svc_end >= NOW() - INTERVAL ? DAY
			UNION ALL
			SELECT svc_availstatus, svc_begin, svc_end FROM services_log_last WHERE svc_id = ?
		) t
		ORDER BY svc_begin`
	return oDb.statusPeriods(ctx, query, false, svcID, days, svcID)
}

// InstanceStatusPeriods returns the availability and overall status periods of
// an instance ending in the last days: the closed ones of svcmon_log, then the
// current one of svcmon_log_last, oldest first.
func (oDb *DB) InstanceStatusPeriods(ctx context.Context, svcID, nodeID string, days int) ([]StatusPeriod, error) {
	query := `SELECT mon_availstatus, mon_overallstatus, DATE_FORMAT(mon_begin, ` + periodDateFormat + `), DATE_FORMAT(mon_end, ` + periodDateFormat + `)
		FROM (
			SELECT mon_availstatus, mon_overallstatus, mon_begin, mon_end FROM svcmon_log
				WHERE svc_id = ? AND node_id = ? AND mon_end >= NOW() - INTERVAL ? DAY
			UNION ALL
			SELECT mon_availstatus, mon_overallstatus, mon_begin, mon_end FROM svcmon_log_last
				WHERE svc_id = ? AND node_id = ?
		) t
		ORDER BY mon_begin`
	return oDb.statusPeriods(ctx, query, true, svcID, nodeID, days, svcID, nodeID)
}

func (oDb *DB) statusPeriods(ctx context.Context, query string, withOverall bool, args ...any) ([]StatusPeriod, error) {
	rows, err := oDb.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("statusPeriods: %w", err)
	}
	defer func() { _ = rows.Close() }()
	periods := []StatusPeriod{}
	for rows.Next() {
		var p StatusPeriod
		var err error
		if withOverall {
			err = rows.Scan(&p.Avail, &p.Overall, &p.Begin, &p.End)
		} else {
			err = rows.Scan(&p.Avail, &p.Begin, &p.End)
		}
		if err != nil {
			return nil, fmt.Errorf("statusPeriods: %w", err)
		}
		periods = append(periods, p)
	}
	return periods, rows.Err()
}

// StatusAck is the justification of a period of a service: why it was not
// available, by whom and when, and whether it still counts in the availability.
type StatusAck struct {
	Begin   string
	End     string
	Comment string
	Account bool
	By      string
	On      string
}

// ServiceStatusAcks returns the justified periods of a service ending in the
// last days, oldest first.
func (oDb *DB) ServiceStatusAcks(ctx context.Context, svcID string, days int) ([]StatusAck, error) {
	query := `SELECT DATE_FORMAT(mon_begin, ` + periodDateFormat + `), DATE_FORMAT(mon_end, ` + periodDateFormat + `),
			mon_comment, mon_account, mon_acked_by, DATE_FORMAT(mon_acked_on, ` + periodDateFormat + `)
		FROM svcmon_log_ack
		WHERE svc_id = ? AND mon_end >= NOW() - INTERVAL ? DAY
		ORDER BY mon_begin`
	rows, err := oDb.DB.QueryContext(ctx, query, svcID, days)
	if err != nil {
		return nil, fmt.Errorf("serviceStatusAcks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	acks := []StatusAck{}
	for rows.Next() {
		var a StatusAck
		if err := rows.Scan(&a.Begin, &a.End, &a.Comment, &a.Account, &a.By, &a.On); err != nil {
			return nil, fmt.Errorf("serviceStatusAcks: %w", err)
		}
		acks = append(acks, a)
	}
	return acks, rows.Err()
}

// SetServiceStatusAck justifies a period of a service, or changes its
// justification, the period being named by its bounds.
func (oDb *DB) SetServiceStatusAck(ctx context.Context, svcID, begin, end, comment string, account bool, by string) error {
	const query = `INSERT INTO svcmon_log_ack (svc_id, mon_begin, mon_end, mon_comment, mon_account, mon_acked_by, mon_acked_on)
		VALUES (?, ?, ?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE mon_comment = VALUES(mon_comment), mon_account = VALUES(mon_account),
			mon_acked_by = VALUES(mon_acked_by), mon_acked_on = NOW()`
	if _, err := oDb.DB.ExecContext(ctx, query, svcID, begin, end, comment, account, by); err != nil {
		return fmt.Errorf("setServiceStatusAck: %w", err)
	}
	oDb.SetChange("svcmon_log_ack")
	return nil
}

// DeleteServiceStatusAck removes the justification of a period of a service;
// found is false when the period was not justified.
func (oDb *DB) DeleteServiceStatusAck(ctx context.Context, svcID, begin, end string) (bool, error) {
	result, err := oDb.DB.ExecContext(ctx,
		"DELETE FROM svcmon_log_ack WHERE svc_id = ? AND mon_begin = ? AND mon_end = ?", svcID, begin, end)
	if err != nil {
		return false, fmt.Errorf("deleteServiceStatusAck: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("deleteServiceStatusAck: %w", err)
	}
	if n > 0 {
		oDb.SetChange("svcmon_log_ack")
	}
	return n > 0, nil
}

// ServicesStatusLog returns, by service, the availability periods and the
// justifications ending in the last days, for a page of services at once.
func (oDb *DB) ServicesStatusLog(ctx context.Context, svcIDs []string, days int) (map[string][]StatusPeriod, map[string][]StatusAck, error) {
	periods := make(map[string][]StatusPeriod, len(svcIDs))
	acks := make(map[string][]StatusAck, len(svcIDs))
	if len(svcIDs) == 0 {
		return periods, acks, nil
	}
	in := Placeholders(len(svcIDs))
	ids := make([]any, len(svcIDs))
	for i, id := range svcIDs {
		ids[i] = id
	}
	query := `SELECT svc_id, svc_availstatus, DATE_FORMAT(svc_begin, ` + periodDateFormat + `), DATE_FORMAT(svc_end, ` + periodDateFormat + `)
		FROM (
			SELECT svc_id, svc_availstatus, svc_begin, svc_end FROM services_log
				WHERE svc_id IN (` + in + `) AND svc_end >= NOW() - INTERVAL ? DAY
			UNION ALL
			SELECT svc_id, svc_availstatus, svc_begin, svc_end FROM services_log_last
				WHERE svc_id IN (` + in + `)
		) t
		ORDER BY svc_id, svc_begin`
	args := append(append(append([]any{}, ids...), days), ids...)
	rows, err := oDb.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("servicesStatusLog: %w", err)
	}
	for rows.Next() {
		var svcID string
		var p StatusPeriod
		if err := rows.Scan(&svcID, &p.Avail, &p.Begin, &p.End); err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("servicesStatusLog: %w", err)
		}
		periods[svcID] = append(periods[svcID], p)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("servicesStatusLog: %w", err)
	}

	ackQuery := `SELECT svc_id, DATE_FORMAT(mon_begin, ` + periodDateFormat + `), DATE_FORMAT(mon_end, ` + periodDateFormat + `), mon_account
		FROM svcmon_log_ack
		WHERE svc_id IN (` + in + `) AND mon_end >= NOW() - INTERVAL ? DAY`
	ackRows, err := oDb.DB.QueryContext(ctx, ackQuery, append(append([]any{}, ids...), days)...)
	if err != nil {
		return nil, nil, fmt.Errorf("servicesStatusLog acks: %w", err)
	}
	defer func() { _ = ackRows.Close() }()
	for ackRows.Next() {
		var svcID string
		var a StatusAck
		if err := ackRows.Scan(&svcID, &a.Begin, &a.End, &a.Account); err != nil {
			return nil, nil, fmt.Errorf("servicesStatusLog acks: %w", err)
		}
		acks[svcID] = append(acks[svcID], a)
	}
	return periods, acks, ackRows.Err()
}

// ServiceAvailability is the stored availability of a service: its rate, and the
// change of that rate over the trend period, in points; nil when unknown.
type ServiceAvailability struct {
	Rate  *float64
	Trend *float64
}

// SetServicesAvailability stores the availability rate of services and its
// trend, nil clearing them, with the time they were computed. The rates are
// stored by one statement, hence one commit, whatever their number: a statement
// and a commit by service could not store the rates of tens of thousands of
// services within the period of the task.
func (oDb *DB) SetServicesAvailability(ctx context.Context, rates map[string]ServiceAvailability) error {
	if len(rates) == 0 {
		return nil
	}
	query, args := servicesAvailabilityUpdate(rates)
	if _, err := oDb.DB.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("setServicesAvailability: %w", err)
	}
	return nil
}

// servicesAvailabilityUpdate returns the UPDATE storing the rates, and its
// arguments. It only updates the services still there: a service purged
// meanwhile is not created again. The services are given in the order of their
// svc_id, for the rows to be locked in the same order by concurrent updates.
func servicesAvailabilityUpdate(rates map[string]ServiceAvailability) (string, []any) {
	value := func(v *float64) any {
		if v == nil {
			return nil
		}
		return *v
	}
	ids := slices.Sorted(maps.Keys(rates))
	rows := make([][]any, len(ids))
	for i, id := range ids {
		rows[i] = []any{id, value(rates[id].Rate), value(rates[id].Trend)}
	}
	given, args := givenRows([]string{"svc_id", "svc_availability", "svc_availability_trend"}, rows)
	query := "UPDATE services s JOIN (" + given + ") r ON s.svc_id = r.svc_id" +
		" SET s.svc_availability = r.svc_availability, s.svc_availability_trend = r.svc_availability_trend, s.svc_availability_updated = NOW()"
	return query, args
}

// ServiceIDs returns the id of every service.
func (oDb *DB) ServiceIDs(ctx context.Context) ([]string, error) {
	rows, err := oDb.DB.QueryContext(ctx, "SELECT svc_id FROM services")
	if err != nil {
		return nil, fmt.Errorf("serviceIDs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("serviceIDs: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
