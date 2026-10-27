package store

import (
	"context"
	"testing"
	"time"
)

func TestDevicesFindAccountsOfOnePerson(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now()
	a, b, c := mkUser(t, s, "a"), mkUser(t, s, "b"), mkUser(t, s, "c")
	dev, ip := "dev"+a.ID, "ip"+a.ID
	day := 24 * time.Hour
	deviceSince, ipSince := now.Add(-90*day), now.Add(-3*day)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.TouchDevices(ctx, a.ID, []DeviceMark{{DeviceCookie, dev}, {DeviceIP, ip}}, now))
	must(s.TouchDevices(ctx, b.ID, []DeviceMark{{DeviceIP, ip}}, now))                    // the same address
	must(s.TouchDevices(ctx, c.ID, []DeviceMark{{DeviceCookie, "other" + a.ID}}, now)) // nothing in common

	if ok, err := s.ShareDevice(ctx, a.ID, b.ID, deviceSince, ipSince); err != nil || !ok {
		t.Errorf("a and b share the address: %v %v", ok, err)
	}
	if ok, _ := s.ShareDevice(ctx, a.ID, c.ID, deviceSince, ipSince); ok {
		t.Error("a and c share nothing")
	}
	// c is also seen with a's cookie, ten days ago: the cookie counts for months, an address for days
	must(s.TouchDevices(ctx, c.ID, []DeviceMark{{DeviceCookie, dev}}, now.Add(-10*day)))
	if ok, _ := s.ShareDevice(ctx, a.ID, c.ID, now.Add(-90*day), ipSince); !ok {
		t.Error("a and c share the cookie")
	}
	ids, err := s.SharingUsers(ctx, a.ID, deviceSince, ipSince)
	if err != nil || len(ids) != 2 {
		t.Errorf("sharing a: %v %v", ids, err)
	}
	// b was last on the address five days ago: not fresh any more
	must(s.TouchDevices(ctx, b.ID, []DeviceMark{{DeviceIP, ip}}, now.Add(-5*day)))
	if ok, _ := s.ShareDevice(ctx, a.ID, b.ID, deviceSince, ipSince); ok {
		t.Error("b was last seen there five days ago")
	}
}

func TestMergeMovesDevices(t *testing.T) {
	s := openTest(t)
	ctx, now := context.Background(), time.Now()
	guest, acc := mkUser(t, s, "guest"), mkUser(t, s, "acc")
	if err := s.TouchDevices(ctx, guest.ID, []DeviceMark{{DeviceCookie, "mrg" + guest.ID}}, now); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeUsers(ctx, guest.ID, acc.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_devices WHERE user_id = ? AND hash = ?", acc.ID, "mrg"+guest.ID).Scan(&n)
	if n != 1 {
		t.Errorf("the account has %d of the guest's devices", n)
	}
}
