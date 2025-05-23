// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/application/testing"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	coremachinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/linklayerdevice"
	portstate "github.com/juju/juju/domain/port/state"
	"github.com/juju/juju/domain/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type unitStateSuite struct {
	baseSuite

	state *State
}

var _ = gc.Suite(&unitStateSuite{})

func (s *unitStateSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *unitStateSuite) assertContainerAddressValues(
	c *gc.C,
	unitName, providerID, addressValue string,
	addressType ipaddress.AddressType,
	addressOrigin ipaddress.Origin,
	addressScope ipaddress.Scope,
	configType ipaddress.ConfigType,

) {
	var (
		gotProviderID string
		gotValue      string
		gotType       int
		gotOrigin     int
		gotScope      int
		gotConfigType int
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `

SELECT cc.provider_id, a.address_value, a.type_id, a.origin_id,a.scope_id,a.config_type_id
FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
JOIN link_layer_device lld ON lld.net_node_uuid = u.net_node_uuid
JOIN ip_address a ON a.device_uuid = lld.uuid
WHERE u.name=?`,

			unitName).Scan(
			&gotProviderID,
			&gotValue,
			&gotType,
			&gotOrigin,
			&gotScope,
			&gotConfigType,
		)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotProviderID, gc.Equals, providerID)
	c.Assert(gotValue, gc.Equals, addressValue)
	c.Assert(gotType, gc.Equals, int(addressType))
	c.Assert(gotOrigin, gc.Equals, int(addressOrigin))
	c.Assert(gotScope, gc.Equals, int(addressScope))
	c.Assert(gotConfigType, gc.Equals, int(configType))
}

func (s *unitStateSuite) assertContainerPortValues(c *gc.C, unitName string, ports []string) {
	var gotPorts []string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `

SELECT ccp.port
FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
JOIN k8s_pod_port ccp ON ccp.unit_uuid = cc.unit_uuid
WHERE u.name=?`,

			unitName)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var port string
			if err := rows.Scan(&port); err != nil {
				return err
			}
			gotPorts = append(gotPorts, port)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return rows.Close()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotPorts, jc.SameContents, ports)
}

func (s *unitStateSuite) TestUpdateCAASUnitCloudContainer(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
					VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
	}
	s.createApplication(c, "foo", life.Alive, u)

	err := s.state.UpdateCAASUnit(context.Background(), "foo/667", application.UpdateCAASUnitParams{})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)

	cc := application.UpdateCAASUnitParams{
		ProviderID: ptr("another-id"),
		Ports:      ptr([]string{"666", "667"}),
		Address:    ptr("2001:db8::1"),
	}
	err = s.state.UpdateCAASUnit(context.Background(), "foo/666", cc)
	c.Assert(err, jc.ErrorIsNil)

	var (
		providerId string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `

SELECT provider_id FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
WHERE u.name=?`,

			"foo/666").Scan(&providerId)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerId, gc.Equals, "another-id")

	s.assertContainerAddressValues(c, "foo/666", "another-id", "2001:db8::1",
		ipaddress.AddressTypeIPv6, ipaddress.OriginProvider, ipaddress.ScopeMachineLocal, ipaddress.ConfigTypeDHCP)
	s.assertContainerPortValues(c, "foo/666", []string{"666", "667"})
}

func (s *unitStateSuite) TestUpdateCAASUnitStatuses(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "some-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
					VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
	}
	s.createApplication(c, "foo", life.Alive, u)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	now := ptr(time.Now())
	params := application.UpdateCAASUnitParams{
		AgentStatus: ptr(status.StatusInfo[status.UnitAgentStatusType]{
			Status:  status.UnitAgentStatusIdle,
			Message: "agent status",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   now,
		}),
		WorkloadStatus: ptr(status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusWaiting,
			Message: "workload status",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   now,
		}),
		K8sPodStatus: ptr(status.StatusInfo[status.K8sPodStatusType]{
			Status:  status.K8sPodStatusRunning,
			Message: "container status",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   now,
		}),
	}
	err = s.state.UpdateCAASUnit(context.Background(), "foo/666", params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "unit_agent", unitUUID, int(status.UnitAgentStatusIdle), "agent status", now, []byte(`{"foo": "bar"}`),
	)
	s.assertUnitStatus(
		c, "unit_workload", unitUUID, int(status.WorkloadStatusWaiting), "workload status", now, []byte(`{"foo": "bar"}`),
	)
	s.assertUnitStatus(
		c, "k8s_pod", unitUUID, int(status.K8sPodStatusRunning), "container status", now, []byte(`{"foo": "bar"}`),
	)
}

func (s *unitStateSuite) TestRegisterCAASUnit(c *gc.C) {
	s.createScalingApplication(c, "foo", life.Alive, 1)

	p := application.RegisterCAASUnitArg{
		UnitName:         "foo/666",
		PasswordHash:     "passwordhash",
		ProviderID:       "some-id",
		Address:          ptr("10.6.6.6"),
		Ports:            ptr([]string{"666"}),
		OrderedScale:     true,
		OrderedId:        0,
		StorageParentDir: c.MkDir(),
	}
	err := s.state.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(err, jc.ErrorIsNil)
	s.assertCAASUnit(c, "foo/666", "passwordhash", "10.6.6.6", []string{"666"})
}

func (s *unitStateSuite) assertCAASUnit(c *gc.C, name, passwordHash, addressValue string, ports []string) {
	var (
		gotPasswordHash  string
		gotAddress       string
		gotAddressType   ipaddress.AddressType
		gotAddressScope  ipaddress.Scope
		gotAddressOrigin ipaddress.Origin
		gotPorts         []string
	)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM unit WHERE name = ?", name).Scan(&gotPasswordHash)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `
SELECT address_value, type_id, scope_id, origin_id FROM ip_address ipa
JOIN link_layer_device lld ON lld.uuid = ipa.device_uuid
JOIN unit u ON u.net_node_uuid = lld.net_node_uuid WHERE u.name = ?
`, name).
			Scan(&gotAddress, &gotAddressType, &gotAddressScope, &gotAddressOrigin)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `
SELECT port FROM k8s_pod_port ccp
JOIN k8s_pod cc ON cc.unit_uuid = ccp.unit_uuid
JOIN unit u ON u.uuid = cc.unit_uuid WHERE u.name = ?
`, name)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var port string
			err = rows.Scan(&port)
			if err != nil {
				return err
			}
			gotPorts = append(gotPorts, port)
		}
		return rows.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotPasswordHash, gc.Equals, passwordHash)
	c.Check(gotAddress, gc.Equals, addressValue)
	c.Check(gotAddressType, gc.Equals, ipaddress.AddressTypeIPv4)
	c.Check(gotAddressScope, gc.Equals, ipaddress.ScopeMachineLocal)
	c.Check(gotAddressOrigin, gc.Equals, ipaddress.OriginProvider)
	c.Check(gotPorts, jc.DeepEquals, ports)
}

func (s *unitStateSuite) TestRegisterCAASUnitAlreadyExists(c *gc.C) {
	unitName := coreunit.Name("foo/0")

	_ = s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: unitName,
	})

	p := application.RegisterCAASUnitArg{
		UnitName:         unitName,
		PasswordHash:     "passwordhash",
		ProviderID:       "some-id",
		Address:          ptr("10.6.6.6"),
		Ports:            ptr([]string{"666"}),
		OrderedScale:     true,
		OrderedId:        0,
		StorageParentDir: c.MkDir(),
	}
	err := s.state.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)

	var (
		providerId   string
		passwordHash string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(ctx, `
SELECT provider_id FROM k8s_pod cc
JOIN unit u ON cc.unit_uuid = u.uuid
WHERE u.name=?`,
			"foo/0").Scan(&providerId)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, `
SELECT password_hash FROM unit
WHERE unit.name=?`,
			"foo/0").Scan(&passwordHash)

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(providerId, gc.Equals, "some-id")
	c.Check(passwordHash, gc.Equals, "passwordhash")
}

func (s *unitStateSuite) TestRegisterCAASUnitReplaceDead(c *gc.C) {
	s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", "foo/0")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	p := application.RegisterCAASUnitArg{
		UnitName:         coreunit.Name("foo/0"),
		PasswordHash:     "passwordhash",
		ProviderID:       "foo-0",
		Address:          ptr("10.6.6.6"),
		Ports:            ptr([]string{"666"}),
		OrderedScale:     true,
		OrderedId:        0,
		StorageParentDir: c.MkDir(),
	}
	err = s.state.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitAlreadyExists)
}

func (s *unitStateSuite) TestRegisterCAASUnitApplicationNotALive(c *gc.C) {
	s.createApplication(c, "foo", life.Dying, application.InsertUnitArg{
		UnitName: "foo/0",
	})
	p := application.RegisterCAASUnitArg{
		UnitName:         "foo/0",
		PasswordHash:     "passwordhash",
		ProviderID:       "foo-0",
		Address:          ptr("10.6.6.6"),
		Ports:            ptr([]string{"666"}),
		OrderedScale:     true,
		OrderedId:        0,
		StorageParentDir: c.MkDir(),
	}

	err := s.state.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSuite) TestRegisterCAASUnitExceedsScale(c *gc.C) {
	appUUID := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE application_scale
SET scale = ?, scale_target = ?
WHERE application_uuid = ?`, 1, 3, appUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	p := application.RegisterCAASUnitArg{
		UnitName:         "foo/2",
		PasswordHash:     "passwordhash",
		ProviderID:       "foo-2",
		Address:          ptr("10.6.6.6"),
		Ports:            ptr([]string{"666"}),
		OrderedScale:     true,
		OrderedId:        2,
		StorageParentDir: c.MkDir(),
	}

	err = s.state.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *unitStateSuite) TestRegisterCAASUnitExceedsScaleTarget(c *gc.C) {
	appUUID := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
	})

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE application_scale
SET scaling = ?, scale = ?, scale_target = ?
WHERE application_uuid = ?`, true, 3, 1, appUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	p := application.RegisterCAASUnitArg{
		UnitName:         "foo/2",
		PasswordHash:     "passwordhash",
		ProviderID:       "foo-2",
		Address:          ptr("10.6.6.6"),
		Ports:            ptr([]string{"666"}),
		OrderedScale:     true,
		OrderedId:        2,
		StorageParentDir: c.MkDir(),
	}

	err = s.state.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotAssigned)
}

func (s *unitStateSuite) TestGetUnitLife(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	unitLife, err := s.state.GetUnitLife(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitLife, gc.Equals, life.Alive)
}

func (s *unitStateSuite) TestGetUnitLifeNotFound(c *gc.C) {
	_, err := s.state.GetUnitLife(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestSetUnitLife(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	ctx := context.Background()
	s.createApplication(c, "foo", life.Alive, u)

	checkResult := func(want life.Life) {
		var gotLife life.Life
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, "SELECT life_id FROM unit WHERE name=?", u.UnitName).
				Scan(&gotLife)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(gotLife, jc.DeepEquals, want)
	}

	err := s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dying)

	err = s.state.SetUnitLife(ctx, "foo/666", life.Dead)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)

	// Can't go backwards.
	err = s.state.SetUnitLife(ctx, "foo/666", life.Dying)
	c.Assert(err, jc.ErrorIsNil)
	checkResult(life.Dead)
}

func (s *unitStateSuite) TestSetUnitLifeNotFound(c *gc.C) {
	err := s.state.SetUnitLife(context.Background(), "foo/666", life.Dying)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestDeleteUnit(c *gc.C) {
	// TODO(units) - add references to agents etc when those are fully cooked
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
		CloudContainer: &application.CloudContainer{
			ProviderID: "provider-id",
			Ports:      ptr([]string{"666", "668"}),
			Address: ptr(application.ContainerAddress{
				Device: application.ContainerDevice{
					Name:              "placeholder",
					DeviceTypeID:      linklayerdevice.DeviceTypeUnknown,
					VirtualPortTypeID: linklayerdevice.NonVirtualPortType,
				},
				Value:       "10.6.6.6",
				AddressType: ipaddress.AddressTypeIPv4,
				ConfigType:  ipaddress.ConfigTypeDHCP,
				Scope:       ipaddress.ScopeMachineLocal,
				Origin:      ipaddress.OriginHost,
			}),
		},
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusExecuting,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   ptr(time.Now()),
			},
			WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   ptr(time.Now()),
			},
		},
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)
	var (
		unitUUID    coreunit.UUID
		netNodeUUID string
		deviceUUID  string
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT uuid, net_node_uuid FROM unit WHERE name=?", u1.UnitName).Scan(&unitUUID, &netNodeUUID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM link_layer_device WHERE net_node_uuid=?", netNodeUUID).Scan(&deviceUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.state.setK8sPodStatus(ctx, tx, unitUUID, &status.StatusInfo[status.K8sPodStatusType]{
			Status:  status.K8sPodStatusRunning,
			Message: "test",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   ptr(time.Now()),
		}); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	portSt := portstate.NewState(s.TxnRunnerFactory())
	err = portSt.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"endpoint": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
		"misc": {
			{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	gotIsLast, err := s.state.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotIsLast, jc.IsFalse)

	var (
		unitCount                 int
		containerCount            int
		deviceCount               int
		addressCount              int
		portCount                 int
		agentStatusCount          int
		workloadStatusCount       int
		cloudContainerStatusCount int
		unitConstraintCount       int
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).Scan(&unitCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM k8s_pod WHERE unit_uuid=?", unitUUID).Scan(&containerCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM link_layer_device WHERE net_node_uuid=?", netNodeUUID).Scan(&deviceCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM ip_address WHERE device_uuid=?", deviceUUID).Scan(&addressCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM k8s_pod_port WHERE unit_uuid=?", unitUUID).Scan(&portCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_agent_status WHERE unit_uuid=?", unitUUID).Scan(&agentStatusCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_workload_status WHERE unit_uuid=?", unitUUID).Scan(&workloadStatusCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM k8s_pod_status WHERE unit_uuid=?", unitUUID).Scan(&cloudContainerStatusCount); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit_constraint WHERE unit_uuid=?", unitUUID).Scan(&unitConstraintCount); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(addressCount, gc.Equals, 0)
	c.Check(portCount, gc.Equals, 0)
	c.Check(deviceCount, gc.Equals, 0)
	c.Check(containerCount, gc.Equals, 0)
	c.Check(agentStatusCount, gc.Equals, 0)
	c.Check(workloadStatusCount, gc.Equals, 0)
	c.Check(cloudContainerStatusCount, gc.Equals, 0)
	c.Check(unitCount, gc.Equals, 0)
	c.Check(unitConstraintCount, gc.Equals, 0)
}

func (s *unitStateSuite) TestDeleteUnitLastUnitAppAlive(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	gotIsLast, err := s.state.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotIsLast, jc.IsFalse)

	var unitCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).
			Scan(&unitCount); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitCount, gc.Equals, 0)
}

func (s *unitStateSuite) TestDeleteUnitLastUnit(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Dying, u1)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id=2 WHERE name=?", u1.UnitName); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	gotIsLast, err := s.state.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotIsLast, jc.IsTrue)

	var unitCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE name=?", u1.UnitName).
			Scan(&unitCount); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitCount, gc.Equals, 0)
}

func (s *unitStateSuite) TestGetUnitUUIDByName(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	_ = s.createApplication(c, "foo", life.Alive, u1)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitUUID, gc.NotNil)
}

func (s *unitStateSuite) TestGetUnitUUIDByNameNotFound(c *gc.C) {
	_, err := s.state.GetUnitUUIDByName(context.Background(), "failme")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) assertUnitStatus(c *gc.C, statusType, unitUUID coreunit.UUID, statusID int, message string, since *time.Time, data []byte) {
	var (
		gotStatusID int
		gotMessage  string
		gotSince    *time.Time
		gotData     []byte
	)
	queryInfo := fmt.Sprintf(`SELECT status_id, message, data, updated_at FROM %s_status WHERE unit_uuid = ?`, statusType)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, queryInfo, unitUUID).
			Scan(&gotStatusID, &gotMessage, &gotData, &gotSince); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatusID, gc.Equals, statusID)
	c.Check(gotMessage, gc.Equals, message)
	c.Check(gotSince, jc.DeepEquals, since)
	c.Check(gotData, jc.DeepEquals, data)
}

func (s *unitStateSuite) TestAddUnitsApplicationNotFound(c *gc.C) {
	u := application.AddUnitArg{
		UnitName: "foo/666",
	}
	uuid := testing.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)
	err := s.state.AddIAASUnits(context.Background(), c.MkDir(), uuid, charmUUID, u)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestAddUnitsApplicationNotAlive(c *gc.C) {
	appID := s.createApplication(c, "foo", life.Dying)

	charmUUID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	u := application.AddUnitArg{
		UnitName: "foo/666",
	}
	err = s.state.AddIAASUnits(context.Background(), c.MkDir(), appID, charmUUID, u)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSuite) TestAddIAASUnits(c *gc.C) {
	s.assertAddUnits(c, model.IAAS)
}

func (s *unitStateSuite) TestAddCAASUnits(c *gc.C) {
	s.assertAddUnits(c, model.CAAS)
}

func (s *unitStateSuite) assertAddUnits(c *gc.C, modelType model.ModelType) {
	appID := s.createApplication(c, "foo", life.Alive)

	charmUUID, err := s.state.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	now := ptr(time.Now())
	u := application.AddUnitArg{
		UnitName: "foo/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusExecuting,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   now,
			},
			WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "test",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   now,
			},
		},
	}

	if modelType == model.IAAS {
		err = s.state.AddIAASUnits(context.Background(), c.MkDir(), appID, charmUUID, u)
	} else {
		err = s.state.AddCAASUnits(context.Background(), c.MkDir(), appID, charmUUID, u)
	}
	c.Assert(err, jc.ErrorIsNil)

	var (
		unitUUID, unitName string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid, name FROM unit WHERE application_uuid=?", appID).Scan(&unitUUID, &unitName)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitName, gc.Equals, "foo/666")
	s.assertUnitStatus(
		c, "unit_agent", coreunit.UUID(unitUUID),
		int(u.UnitStatusArg.AgentStatus.Status), u.UnitStatusArg.AgentStatus.Message,
		u.UnitStatusArg.AgentStatus.Since, u.UnitStatusArg.AgentStatus.Data)
	s.assertUnitStatus(
		c, "unit_workload", coreunit.UUID(unitUUID),
		int(u.UnitStatusArg.WorkloadStatus.Status), u.UnitStatusArg.WorkloadStatus.Message,
		u.UnitStatusArg.WorkloadStatus.Since, u.UnitStatusArg.WorkloadStatus.Data)
	s.assertUnitConstraints(c, coreunit.UUID(unitUUID), constraints.Constraints{})
}

func (s *unitStateSuite) TestInitialWatchStatementUnitLife(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	var unitID1, unitID2 string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&unitID1); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	table, queryFunc := s.state.InitialWatchStatementUnitLife("foo")
	c.Assert(table, gc.Equals, "unit")

	result, err := queryFunc(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []string{unitID1, unitID2})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributes(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	cc := application.UpdateCAASUnitParams{
		ProviderID: ptr("another-id"),
		Ports:      ptr([]string{"666", "667"}),
		Address:    ptr("2001:db8::1"),
	}
	err := s.state.UpdateCAASUnit(context.Background(), "foo/666", cc)
	c.Assert(err, jc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(refreshAttributes, gc.DeepEquals, application.UnitAttributes{
		Life:        life.Alive,
		ProviderID:  "another-id",
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesNoProviderID(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(refreshAttributes, gc.DeepEquals, application.UnitAttributes{
		Life:        life.Alive,
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesWithResolveMode(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	unitUUID, err := s.state.GetUnitUUIDByName(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO unit_resolved (unit_uuid, mode_id) VALUES (?, 0)", unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(refreshAttributes, gc.DeepEquals, application.UnitAttributes{
		Life:        life.Alive,
		ResolveMode: "retry-hooks",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesDeadLife(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name = ?", u.UnitName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(refreshAttributes, gc.DeepEquals, application.UnitAttributes{
		Life:        life.Dead,
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestGetUnitRefreshAttributesDyingLife(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name = ?", u.UnitName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	refreshAttributes, err := s.state.GetUnitRefreshAttributes(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(refreshAttributes, gc.DeepEquals, application.UnitAttributes{
		Life:        life.Dying,
		ResolveMode: "none",
	})
}

func (s *unitStateSuite) TestSetConstraintFull(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.Constraints{
		Arch:             ptr("amd64"),
		CpuCores:         ptr(uint64(2)),
		CpuPower:         ptr(uint64(42)),
		Mem:              ptr(uint64(8)),
		RootDisk:         ptr(uint64(256)),
		RootDiskSource:   ptr("root-disk-source"),
		InstanceRole:     ptr("instance-role"),
		InstanceType:     ptr("instance-type"),
		Container:        ptr(instance.LXD),
		VirtType:         ptr("virt-type"),
		AllocatePublicIP: ptr(true),
		ImageID:          ptr("image-id"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space0", Exclude: false},
			{SpaceName: "space1", Exclude: true},
		}),
		Tags:  ptr([]string{"tag0", "tag1"}),
		Zones: ptr([]string{"zone0", "zone1"}),
	}

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertSpace0Stmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, insertSpace0Stmt, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		insertSpace1Stmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertSpace1Stmt, "space1-uuid", "space1")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetUnitConstraints(context.Background(), unitUUID, cons)
	c.Assert(err, jc.ErrorIsNil)
	constraintSpaces, constraintTags, constraintZones := s.assertUnitConstraints(c, unitUUID, cons)

	c.Check(constraintSpaces, jc.DeepEquals, []applicationSpace{
		{SpaceName: "space0", SpaceExclude: false},
		{SpaceName: "space1", SpaceExclude: true},
	})
	c.Check(constraintTags, jc.DeepEquals, []string{"tag0", "tag1"})
	c.Check(constraintZones, jc.DeepEquals, []string{"zone0", "zone1"})
}

func (s *unitStateSuite) TestSetConstraintInvalidContainerType(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.Constraints{
		Container: ptr(instance.ContainerType("invalid-container-type")),
	}
	err = s.state.SetUnitConstraints(context.Background(), unitUUID, cons)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidUnitConstraints)
}

func (s *unitStateSuite) TestSetConstraintInvalidSpace(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)
	var unitUUID coreunit.UUID
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.Constraints{
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "invalid-space", Exclude: false},
		}),
	}
	err = s.state.SetUnitConstraints(context.Background(), unitUUID, cons)
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidUnitConstraints)
}

func (s *unitStateSuite) TestSetConstraintsUnitNotFound(c *gc.C) {
	err := s.state.SetUnitConstraints(context.Background(), "foo", constraints.Constraints{Mem: ptr(uint64(8))})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetAllUnitNamesNoUnits(c *gc.C) {
	names, err := s.state.GetAllUnitNames(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, jc.DeepEquals, []coreunit.Name{})
}

func (s *unitStateSuite) TestGetAllUnitNames(c *gc.C) {
	s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/666",
	}, application.InsertUnitArg{
		UnitName: "foo/667",
	})
	s.createApplication(c, "bar", life.Alive, application.InsertUnitArg{
		UnitName: "bar/666",
	})

	names, err := s.state.GetAllUnitNames(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, jc.SameContents, []coreunit.Name{"foo/666", "foo/667", "bar/666"})
}

func (s *unitStateSuite) TestGetUnitNamesForApplicationNotFound(c *gc.C) {
	_, err := s.state.GetUnitNamesForApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSuite) TestGetUnitNamesForApplicationDead(c *gc.C) {
	appUUID := s.createApplication(c, "deadapp", life.Dead)
	_, err := s.state.GetUnitNamesForApplication(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *unitStateSuite) TestGetUnitNamesForApplicationNoUnits(c *gc.C) {
	appUUID := s.createApplication(c, "foo", life.Alive)
	names, err := s.state.GetUnitNamesForApplication(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, jc.DeepEquals, []coreunit.Name{})
}

func (s *unitStateSuite) TestGetUnitNamesForApplication(c *gc.C) {
	appUUID := s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/666",
	}, application.InsertUnitArg{
		UnitName: "foo/667",
	})
	s.createApplication(c, "bar", life.Alive, application.InsertUnitArg{
		UnitName: "bar/666",
	})

	names, err := s.state.GetUnitNamesForApplication(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, jc.SameContents, []coreunit.Name{"foo/666", "foo/667"})
}

func (s *unitStateSuite) TestGetUnitNamesForNetNodeNotFound(c *gc.C) {
	_, err := s.state.GetUnitNamesForNetNode(context.Background(), "doink")
	c.Assert(err, jc.ErrorIs, applicationerrors.NetNodeNotFound)
}

func (s *unitStateSuite) TestGetUnitNamesForNetNodeNoUnits(c *gc.C) {
	var netNode string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type: deployment.PlacementTypeUnset,
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netNode, gc.Not(gc.Equals), "")

	names, err := s.state.GetUnitNamesForNetNode(context.Background(), netNode)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, jc.DeepEquals, []coreunit.Name{})
}

func (s *unitStateSuite) TestGetUnitNamesForNetNode(c *gc.C) {
	s.createApplication(c, "foo", life.Alive, application.InsertUnitArg{
		UnitName: "foo/0",
		Placement: deployment.Placement{
			Directive: "0",
		},
	}, application.InsertUnitArg{
		UnitName: "foo/1",
		Placement: deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: "0",
		},
	}, application.InsertUnitArg{
		UnitName: "foo/2",
		Placement: deployment.Placement{
			Directive: "1",
		},
	})

	netNodeUUID, err := s.state.GetMachineNetNodeUUIDFromName(context.Background(), "0")
	c.Assert(err, jc.ErrorIsNil)

	names, err := s.state.GetUnitNamesForNetNode(context.Background(), netNodeUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, jc.DeepEquals, []coreunit.Name{"foo/0", "foo/1"})
}

func (s *unitStateSuite) TestGetUnitWorkloadVersion(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	workloadVersion, err := s.state.GetUnitWorkloadVersion(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloadVersion, gc.Equals, "")
}

func (s *unitStateSuite) TestGetUnitWorkloadVersionNotFound(c *gc.C) {
	_, err := s.state.GetUnitWorkloadVersion(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestSetUnitWorkloadVersion(c *gc.C) {
	u := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u)

	err := s.state.SetUnitWorkloadVersion(context.Background(), u.UnitName, "v1.0.0")
	c.Assert(err, jc.ErrorIsNil)

	workloadVersion, err := s.state.GetUnitWorkloadVersion(context.Background(), u.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloadVersion, gc.Equals, "v1.0.0")
}

func (s *unitStateSuite) TestSetUnitWorkloadVersionMultiple(c *gc.C) {
	u1 := application.InsertUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.InsertUnitArg{
		UnitName: "foo/667",
	}
	appID := s.createApplication(c, "foo", life.Alive, u1, u2)

	s.assertApplicationWorkloadVersion(c, appID, "")

	err := s.state.SetUnitWorkloadVersion(context.Background(), u1.UnitName, "v1.0.0")
	c.Assert(err, jc.ErrorIsNil)

	s.assertApplicationWorkloadVersion(c, appID, "v1.0.0")

	err = s.state.SetUnitWorkloadVersion(context.Background(), u2.UnitName, "v2.0.0")
	c.Assert(err, jc.ErrorIsNil)

	s.assertApplicationWorkloadVersion(c, appID, "v2.0.0")

	workloadVersion, err := s.state.GetUnitWorkloadVersion(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloadVersion, gc.Equals, "v1.0.0")

	workloadVersion, err = s.state.GetUnitWorkloadVersion(context.Background(), u2.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloadVersion, gc.Equals, "v2.0.0")

	s.assertApplicationWorkloadVersion(c, appID, "v2.0.0")
}

func (s *unitStateSuite) TestGetUnitMachineUUID(c *gc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createApplication(c, "foo", life.Alive)
	unitUUID := s.addUnit(c, unitName, appUUID)
	_, machineUUID := s.addMachineToUnit(c, unitUUID)

	machine, err := s.state.GetUnitMachineUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine, gc.Equals, machineUUID)
}

func (s *unitStateSuite) TestGetUnitMachineUUIDNotAssigned(c *gc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createApplication(c, "foo", life.Alive)
	s.addUnit(c, unitName, appUUID)

	_, err := s.state.GetUnitMachineUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitMachineNotAssigned)
}

func (s *unitStateSuite) TestGetUnitMachineUUIDUnitNotFound(c *gc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")

	_, err := s.state.GetUnitMachineUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitMachineUUIDIsDead(c *gc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createApplication(c, "foo", life.Alive)
	s.addUnitWithLife(c, unitName, appUUID, life.Dead)

	_, err := s.state.GetUnitMachineUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *unitStateSuite) TestGetUnitMachineName(c *gc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createApplication(c, "foo", life.Alive)
	unitUUID := s.addUnit(c, unitName, appUUID)
	machineName, _ := s.addMachineToUnit(c, unitUUID)

	machine, err := s.state.GetUnitMachineName(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine, gc.Equals, machineName)
}

func (s *unitStateSuite) TestGetUnitMachineNameNotAssigned(c *gc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createApplication(c, "foo", life.Alive)
	s.addUnit(c, unitName, appUUID)

	_, err := s.state.GetUnitMachineName(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitMachineNotAssigned)
}

func (s *unitStateSuite) TestGetUnitMachineNameUnitNotFound(c *gc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")

	_, err := s.state.GetUnitMachineName(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitStateSuite) TestGetUnitMachineNameIsDead(c *gc.C) {
	unitName := coreunittesting.GenNewName(c, "foo/666")
	appUUID := s.createApplication(c, "foo", life.Alive)
	s.addUnitWithLife(c, unitName, appUUID, life.Dead)

	_, err := s.state.GetUnitMachineName(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *unitStateSuite) assertApplicationWorkloadVersion(c *gc.C, appID coreapplication.ID, expected string) {
	var version string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT version FROM application_workload_version WHERE application_uuid=?", appID).Scan(&version)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, expected)
}

func (s *unitStateSuite) TestSetUnitWorkloadVersionNotFound(c *gc.C) {
	err := s.state.SetUnitWorkloadVersion(context.Background(), coreunit.Name("foo/666"), "v1.0.0")
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

type applicationSpace struct {
	SpaceName    string `db:"space"`
	SpaceExclude bool   `db:"exclude"`
}

func (s *unitStateSuite) assertUnitConstraints(c *gc.C, inUnitUUID coreunit.UUID, cons constraints.Constraints) ([]applicationSpace, []string, []string) {
	var (
		unitUUID                                                            string
		constraintUUID                                                      string
		constraintSpaces                                                    []applicationSpace
		constraintTags                                                      []string
		constraintZones                                                     []string
		arch, rootDiskSource, instanceRole, instanceType, virtType, imageID sql.NullString
		cpuCores, cpuPower, mem, rootDisk                                   sql.NullInt64
		allocatePublicIP                                                    sql.NullBool
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT unit_uuid, constraint_uuid FROM unit_constraint WHERE unit_uuid=?", inUnitUUID).Scan(&unitUUID, &constraintUUID)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, "SELECT space,exclude FROM constraint_space WHERE constraint_uuid=?", constraintUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var space applicationSpace
			if err := rows.Scan(&space.SpaceName, &space.SpaceExclude); err != nil {
				return err
			}
			constraintSpaces = append(constraintSpaces, space)
		}
		if rows.Err() != nil {
			return rows.Err()
		}

		rows, err = tx.QueryContext(ctx, "SELECT tag FROM constraint_tag WHERE constraint_uuid=?", constraintUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tag string
			if err := rows.Scan(&tag); err != nil {
				return err
			}
			constraintTags = append(constraintTags, tag)
		}
		if rows.Err() != nil {
			return rows.Err()
		}

		rows, err = tx.QueryContext(ctx, "SELECT zone FROM constraint_zone WHERE constraint_uuid=?", constraintUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var zone string
			if err := rows.Scan(&zone); err != nil {
				return err
			}
			constraintZones = append(constraintZones, zone)
		}

		row := tx.QueryRowContext(ctx, `
SELECT arch, cpu_cores, cpu_power, mem, root_disk, root_disk_source, instance_role, instance_type, virt_type, allocate_public_ip, image_id
FROM "constraint"
WHERE uuid=?`, constraintUUID)
		err = row.Err()
		if err != nil {
			return err
		}
		if err := row.Scan(&arch, &cpuCores, &cpuPower, &mem, &rootDisk, &rootDiskSource, &instanceRole, &instanceType, &virtType, &allocatePublicIP, &imageID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(constraintUUID, gc.Not(gc.Equals), "")
	c.Check(unitUUID, gc.Equals, inUnitUUID.String())

	c.Check(arch.String, gc.Equals, deptr(cons.Arch))
	c.Check(uint64(cpuCores.Int64), gc.Equals, deptr(cons.CpuCores))
	c.Check(uint64(cpuPower.Int64), gc.Equals, deptr(cons.CpuPower))
	c.Check(uint64(mem.Int64), gc.Equals, deptr(cons.Mem))
	c.Check(uint64(rootDisk.Int64), gc.Equals, deptr(cons.RootDisk))
	c.Check(rootDiskSource.String, gc.Equals, deptr(cons.RootDiskSource))
	c.Check(instanceRole.String, gc.Equals, deptr(cons.InstanceRole))
	c.Check(instanceType.String, gc.Equals, deptr(cons.InstanceType))
	c.Check(virtType.String, gc.Equals, deptr(cons.VirtType))
	c.Check(allocatePublicIP.Bool, gc.Equals, deptr(cons.AllocatePublicIP))
	c.Check(imageID.String, gc.Equals, deptr(cons.ImageID))

	return constraintSpaces, constraintTags, constraintZones
}

func (s *unitStateSuite) addUnit(c *gc.C, unitName coreunit.Name, appUUID coreapplication.ID) coreunit.UUID {
	return s.addUnitWithLife(c, unitName, appUUID, life.Alive)
}

func (s *unitStateSuite) addUnitWithLife(c *gc.C, unitName coreunit.Name, appUUID coreapplication.ID, l life.Life) coreunit.UUID {
	unitUUID := coreunittesting.GenUnitUUID(c)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		netNodeUUID := uuid.MustNewUUID().String()
		_, err := tx.Exec(`
INSERT INTO net_node (uuid)
VALUES (?)
`, netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO unit (uuid, name, life_id, net_node_uuid, application_uuid, charm_uuid)
SELECT ?, ?, ?, ?, uuid, charm_uuid
FROM application
WHERE uuid = ?
`, unitUUID, unitName, l, netNodeUUID, appUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return unitUUID
}

func (s *unitStateSuite) addMachineToUnit(c *gc.C, unitUUID coreunit.UUID) (coremachine.Name, coremachine.UUID) {
	machineUUID := coremachinetesting.GenUUID(c)
	machineName := coremachine.Name("0")
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO machine (uuid, name, life_id, net_node_uuid)
SELECT ?, ?, ?, net_node_uuid
FROM unit
WHERE uuid = ?
`, machineUUID, machineName, 0 /* alive */, unitUUID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return machineName, machineUUID
}

type unitStateSubordinateSuite struct {
	unitStateSuite
}

var _ = gc.Suite(&unitStateSubordinateSuite{})

func (s *unitStateSubordinateSuite) TestAddSubordinateUnit(c *gc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	s.createApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: pUnitName,
	})

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Act:
	sUnitName, err := s.state.AddSubordinateUnit(context.Background(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
		ModelType:         model.IAAS,
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sUnitName, gc.Equals, coreunittesting.GenNewName(c, "subordinate/0"))
	s.assertUnitPrincipal(c, pUnitName, sUnitName)
	s.assertUnitMachinesMatch(c, pUnitName, sUnitName)
}

// TestAddSubordinateUnitSecondSubordinate tests that a second subordinate unit
// can be added to an app with no issues.
func (s *unitStateSubordinateSuite) TestAddSubordinateUnitSecondSubordinate(c *gc.C) {
	// Arrange: add subordinate application.
	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Arrange: add principal app and add a subordinate unit on it.
	pUnitName1 := coreunittesting.GenNewName(c, "foo/666")
	pUnitName2 := coreunittesting.GenNewName(c, "foo/667")
	s.createApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: pUnitName1,
	}, application.InsertUnitArg{
		UnitName: pUnitName2,
	})
	_, err := s.state.AddSubordinateUnit(context.Background(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName1,
		ModelType:         model.IAAS,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Act: Add a second subordinate unit
	sUnitName2, err := s.state.AddSubordinateUnit(context.Background(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName2,
		ModelType:         model.IAAS,
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sUnitName2, gc.Equals, coreunittesting.GenNewName(c, "subordinate/1"))
	s.assertUnitPrincipal(c, pUnitName2, sUnitName2)
	s.assertUnitMachinesMatch(c, pUnitName2, sUnitName2)
}

func (s *unitStateSubordinateSuite) TestAddSubordinateUnitCAAS(c *gc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	s.createApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: pUnitName,
	})

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Act:
	sUnitName, err := s.state.AddSubordinateUnit(context.Background(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
		ModelType:         model.CAAS,
	})

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sUnitName, gc.Equals, coreunittesting.GenNewName(c, "subordinate/0"))
	s.assertUnitPrincipal(c, pUnitName, sUnitName)
}

func (s *unitStateSubordinateSuite) TestAddSubordinateUnitTwiceToSameUnit(c *gc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	s.createApplication(c, "principal", life.Alive, application.InsertUnitArg{
		UnitName: pUnitName,
	})

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Arrange: Add the first subordinate.
	_, err := s.state.AddSubordinateUnit(context.Background(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
		ModelType:         model.IAAS,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Act: try adding a second subordinate to the same unit.
	_, err = s.state.AddSubordinateUnit(context.Background(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
		ModelType:         model.IAAS,
	})

	// Assert
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitAlreadyHasSubordinate)
}

func (s *unitStateSubordinateSuite) TestAddSubordinateUnitWithoutMachine(c *gc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")
	pAppUUID := s.createApplication(c, "principal", life.Alive)
	s.addUnit(c, pUnitName, pAppUUID)

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Alive)

	// Act:
	_, err := s.state.AddSubordinateUnit(context.Background(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
		ModelType:         model.IAAS,
	})

	// Assert
	c.Assert(err, jc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *unitStateSubordinateSuite) TestAddSubordinateUnitApplicationNotAlive(c *gc.C) {
	// Arrange:
	pUnitName := coreunittesting.GenNewName(c, "foo/666")

	sAppID := s.createSubordinateApplication(c, "subordinate", life.Dying)

	// Act:
	_, err := s.state.AddSubordinateUnit(context.Background(), application.SubordinateUnitArg{
		SubordinateAppID:  sAppID,
		PrincipalUnitName: pUnitName,
		ModelType:         model.IAAS,
	})

	// Assert
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitStateSubordinateSuite) TestIsSubordinateApplication(c *gc.C) {
	// Arrange:
	appID := s.createSubordinateApplication(c, "sub", life.Alive)

	// Act:
	isSub, err := s.state.IsSubordinateApplication(context.Background(), appID)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isSub, jc.IsTrue)
}

func (s *unitStateSubordinateSuite) TestIsSubordinateApplicationFalse(c *gc.C) {
	// Arrange:
	appID := s.createApplication(c, "notSubordinate", life.Alive)

	// Act:
	isSub, err := s.state.IsSubordinateApplication(context.Background(), appID)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isSub, jc.IsFalse)
}

func (s *unitStateSubordinateSuite) TestIsSubordinateApplicationNotFound(c *gc.C) {
	// Act:
	_, err := s.state.IsSubordinateApplication(context.Background(), "notfound")

	// Assert
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitStateSubordinateSuite) TestGetUnitPrincipal(c *gc.C) {
	principalAppID := s.createApplication(c, "principal", life.Alive)
	subAppID := s.createSubordinateApplication(c, "sub", life.Alive)
	principalName := coreunittesting.GenNewName(c, "principal/0")
	subName := coreunittesting.GenNewName(c, "sub/0")
	principalUUID := s.addUnit(c, principalName, principalAppID)
	subUUID := s.addUnit(c, subName, subAppID)
	s.addUnitPrincipal(c, principalUUID, subUUID)

	foundPrincipalName, ok, err := s.state.GetUnitPrincipal(context.Background(), subName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(foundPrincipalName, gc.Equals, principalName)
	c.Check(ok, jc.IsTrue)
}

func (s *unitStateSubordinateSuite) TestGetUnitPrincipalSubordinateNotPrincipal(c *gc.C) {
	principalAppID := s.createApplication(c, "principal", life.Alive)
	subAppID := s.createSubordinateApplication(c, "sub", life.Alive)
	principalName := coreunittesting.GenNewName(c, "principal/0")
	subName := coreunittesting.GenNewName(c, "sub/0")
	s.addUnit(c, principalName, principalAppID)
	s.addUnit(c, subName, subAppID)

	_, ok, err := s.state.GetUnitPrincipal(context.Background(), subName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsFalse)
}

func (s *unitStateSubordinateSuite) TestGetUnitPrincipalNoUnitExists(c *gc.C) {
	subName := coreunittesting.GenNewName(c, "sub/0")

	_, ok, err := s.state.GetUnitPrincipal(context.Background(), subName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsFalse)
}

func (s *unitStateSubordinateSuite) assertUnitMachinesMatch(c *gc.C, unit1, unit2 coreunit.Name) {
	m1 := s.getUnitMachine(c, unit1)
	m2 := s.getUnitMachine(c, unit2)
	c.Assert(m1, gc.Equals, m2)
}

func (s *unitStateSubordinateSuite) getUnitMachine(c *gc.C, unitName coreunit.Name) string {
	var machineName string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {

		err := tx.QueryRow(`
SELECT machine.name
FROM unit
JOIN machine ON unit.net_node_uuid = machine.net_node_uuid
WHERE unit.name = ?
`, unitName).Scan(&machineName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return machineName
}

func (s *unitStateSubordinateSuite) addUnitPrincipal(c *gc.C, principal, sub coreunit.UUID) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO unit_principal (principal_uuid, unit_uuid)
VALUES (?, ?) 
`, principal, sub)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitStateSubordinateSuite) assertUnitPrincipal(c *gc.C, principalName, subordinateName coreunit.Name) {
	var foundPrincipalName coreunit.Name
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT u1.name
FROM unit u1
JOIN unit_principal up ON up.principal_uuid = u1.uuid
JOIN unit u2 ON u2.uuid = up.unit_uuid
WHERE u2.name = ?
`, subordinateName).Scan(&foundPrincipalName)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundPrincipalName, gc.Equals, principalName)
}

func (s *unitStateSubordinateSuite) createSubordinateApplication(c *gc.C, name string, l life.Life) coreapplication.ID {
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	appID, err := state.CreateApplication(context.Background(), name, application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name:        name,
				Subordinate: true,
			},
			Manifest:      s.minimalManifest(c),
			ReferenceName: name,
			Source:        charm.CharmHubSource,
			Revision:      42,
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	return appID
}

func deptr[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}
