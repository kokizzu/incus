package ovs

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-logr/logr"
	ovsdbClient "github.com/ovn-org/libovsdb/client"

	ovsSwitch "github.com/lxc/incus/v6/internal/server/network/ovs/schema/ovs"
)

// VSwitch client.
type VSwitch struct {
	client   ovsdbClient.Client
	cookie   ovsdbClient.MonitorCookie
	rootUUID string
}

// NewVSwitch initializes a new vSwitch client..
func NewVSwitch(dbAddr string) (*VSwitch, error) {
	// Prepare the OVSDB client.
	dbSchema, err := ovsSwitch.FullDatabaseModel()
	if err != nil {
		return nil, err
	}

	discard := logr.Discard()

	options := []ovsdbClient.Option{
		ovsdbClient.WithLogger(&discard),
		ovsdbClient.WithEndpoint(dbAddr),
		ovsdbClient.WithReconnect(5*time.Second, &backoff.ZeroBackOff{}),
	}

	// Connect to OVSDB.
	ovs, err := ovsdbClient.NewOVSDBClient(dbSchema, options...)
	if err != nil {
		return nil, err
	}

	err = ovs.Connect(context.TODO())
	if err != nil {
		return nil, err
	}

	err = ovs.Echo(context.TODO())
	if err != nil {
		return nil, err
	}

	monitorCookie, err := ovs.MonitorAll(context.TODO())
	if err != nil {
		return nil, err
	}

	// Create the SB struct.
	client := &VSwitch{
		client: ovs,
		cookie: monitorCookie,
	}

	// Set finalizer to stop the monitor.
	runtime.SetFinalizer(client, func(o *VSwitch) {
		_ = ovs.MonitorCancel(context.Background(), o.cookie)
		ovs.Close()
	})

	// Get the root UUID.
	rows := ovs.Cache().Table("Open_vSwitch").Rows()
	if len(rows) != 1 {
		return nil, errors.New("Cannot find the OVS root switch")
	}

	for uuid := range rows {
		client.rootUUID = uuid
	}

	return client, nil
}
