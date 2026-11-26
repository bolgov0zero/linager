package store

import (
	"context"
	"time"
)

// Device statuses.
const (
	DevicePending  = "pending"
	DeviceApproved = "approved"
	DeviceDenied   = "denied"
)

// DeviceApprovalSetting holds the time the confirmation of devices was turned on (RFC 3339); empty: it is off.
const DeviceApprovalSetting = "device_approval"

// Device is one installation of an application that signs in to the server.
type Device struct {
	ID         int64
	UserID     int64
	Login      string
	TokenHash  []byte
	Platform   string
	AppVersion string
	OSVersion  string
	Model      string
	Label      string
	Status     string
	LastIP     string
	LastSeen   time.Time
	CreatedAt  time.Time
	DecidedAt  *time.Time
	DecidedBy  string
}

// DeviceInfo is what a client says about itself (the header X-Linager-Client).
type DeviceInfo struct {
	Platform   string
	AppVersion string
	OSVersion  string
	Model      string
}

const deviceCols = `d.id, d.user_id, u.login, d.token_hash, d.platform, d.app_version, d.os_version, d.model, d.label, d.status, d.last_ip, d.last_seen, d.created_at, d.decided_at, coalesce(b.login,'')`
const deviceFrom = ` FROM devices d JOIN users u ON u.id=d.user_id LEFT JOIN users b ON b.id=d.decided_by`

type rowScanner interface{ Scan(dest ...any) error }

func scanDevice(r rowScanner) (*Device, error) {
	var d Device
	err := r.Scan(&d.ID, &d.UserID, &d.Login, &d.TokenHash, &d.Platform, &d.AppVersion, &d.OSVersion, &d.Model, &d.Label, &d.Status, &d.LastIP, &d.LastSeen, &d.CreatedAt, &d.DecidedAt, &d.DecidedBy)
	if err != nil {
		return nil, notFound(err)
	}
	return &d, nil
}

// DeviceByID returns a device.
func (s *Store) DeviceByID(ctx context.Context, id int64) (*Device, error) {
	return scanDevice(s.Pool.QueryRow(ctx, `SELECT `+deviceCols+deviceFrom+` WHERE d.id=$1`, id))
}

// DeviceByToken returns the device of a user that holds the token (by its hash), or ErrNotFound.
func (s *Store) DeviceByToken(ctx context.Context, userID int64, tokenHash []byte) (*Device, error) {
	return scanDevice(s.Pool.QueryRow(ctx, `SELECT `+deviceCols+deviceFrom+` WHERE d.user_id=$1 AND d.token_hash=$2`, userID, tokenHash))
}

// ListDevices returns every device: the ones waiting first, then the newest seen.
func (s *Store) ListDevices(ctx context.Context) ([]Device, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+deviceCols+deviceFrom+` ORDER BY (d.status='pending') DESC, d.last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// RegisterDevice finds the device of a user by its token or makes a new one with the given status; what the client says about itself
// and its address are kept up to date. created tells whether the device is new.
func (s *Store) RegisterDevice(ctx context.Context, userID int64, tokenHash []byte, info DeviceInfo, ip, status string) (*Device, bool, error) {
	var id int64
	var created bool
	err := s.Pool.QueryRow(ctx, `INSERT INTO devices (user_id, token_hash, platform, app_version, os_version, model, status, last_ip, decided_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8, CASE WHEN $7='approved' THEN now() END)
		ON CONFLICT (token_hash, user_id) DO UPDATE SET platform=COALESCE(NULLIF(EXCLUDED.platform,''), devices.platform), app_version=COALESCE(NULLIF(EXCLUDED.app_version,''), devices.app_version),
			os_version=COALESCE(NULLIF(EXCLUDED.os_version,''), devices.os_version), model=COALESCE(NULLIF(EXCLUDED.model,''), devices.model), last_ip=EXCLUDED.last_ip, last_seen=now()
		RETURNING id, (xmax = 0)`, userID, tokenHash, info.Platform, info.AppVersion, info.OSVersion, info.Model, status, ip).Scan(&id, &created)
	if err != nil {
		return nil, false, err
	}
	d, err := s.DeviceByID(ctx, id)
	return d, created, err
}

// TouchDevice keeps the address, the versions and the time of the last request of a device.
func (s *Store) TouchDevice(ctx context.Context, id int64, info DeviceInfo, ip string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE devices SET platform=COALESCE(NULLIF($2,''), platform), app_version=COALESCE(NULLIF($3,''), app_version), os_version=COALESCE(NULLIF($4,''), os_version), model=COALESCE(NULLIF($5,''), model), last_ip=$6, last_seen=now() WHERE id=$1`,
		id, info.Platform, info.AppVersion, info.OSVersion, info.Model, ip)
	return err
}

// SetDeviceStatus approves or denies a device; by is the administrator who decided (0: the command line of the server).
func (s *Store) SetDeviceStatus(ctx context.Context, id int64, status string, by int64) error {
	var byID *int64
	if by > 0 {
		byID = &by
	}
	ct, err := s.Pool.Exec(ctx, `UPDATE devices SET status=$2, decided_at=now(), decided_by=$3 WHERE id=$1`, id, status, byID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	if status == DeviceDenied {
		_, err = s.Pool.Exec(ctx, `DELETE FROM sessions WHERE device_id=$1`, id)
	}
	return err
}

// SetDeviceLabel names a device (the administrator's own name for it).
func (s *Store) SetDeviceLabel(ctx context.Context, id int64, label string) error {
	ct, err := s.Pool.Exec(ctx, `UPDATE devices SET label=$2 WHERE id=$1`, id, label)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDevice removes a device; its sessions go with it (it is signed out).
func (s *Store) DeleteDevice(ctx context.Context, id int64) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM devices WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// BindSession ties a session that has no device yet to a device.
func (s *Store) BindSession(ctx context.Context, sessionID []byte, deviceID int64) error {
	_, err := s.Pool.Exec(ctx, `UPDATE sessions SET device_id=$2 WHERE id=$1 AND device_id IS NULL`, sessionID, deviceID)
	return err
}

// DeleteSessionsExcept signs everybody out but one session (nil: everybody).
func (s *Store) DeleteSessionsExcept(ctx context.Context, keep []byte) error {
	if keep == nil {
		_, err := s.Pool.Exec(ctx, `DELETE FROM sessions`)
		return err
	}
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE id<>$1`, keep)
	return err
}

// PendingDevices counts the devices that wait for a decision.
func (s *Store) PendingDevices(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM devices WHERE status='pending'`).Scan(&n)
	return n, err
}
