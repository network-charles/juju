// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/cert"
	"github.com/juju/worker/v4"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent/engine"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud-controller/agent/agenttest"
	"github.com/juju/juju/cmd/jujud-controller/agent/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	blockdevicestate "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/environs/filestorage"
	envstorage "github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/mongo"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/dbaccessor"
	databasetesting "github.com/juju/juju/internal/worker/dbaccessor/testing"
	"github.com/juju/juju/internal/worker/diskmanager"
	"github.com/juju/juju/internal/worker/machiner"
	"github.com/juju/juju/internal/worker/storageprovisioner"
	"github.com/juju/juju/state"
)

const (
	// Use a longer wait in tests that are dependent on leases - sometimes
	// the raft workers can take a bit longer to spin up.
	longerWait = 2 * coretesting.LongWait

	// This is the address that the raft workers will use for the server.
	serverAddress = "localhost:17070"
)

type MachineSuite struct {
	commonMachineSuite

	agentStorage envstorage.Storage
}

var _ = gc.Suite(&MachineSuite{})

// DefaultVersions returns a slice of unique 'versions' for the current
// environment's host architecture. Additionally, it ensures that 'versions'
// for amd64 are returned if that is not the current host's architecture.
func defaultVersions(agentVersion semversion.Number) []semversion.Binary {
	osTypes := set.NewStrings("ubuntu")
	osTypes.Add(coreos.HostOSTypeName())
	var versions []semversion.Binary
	for _, osType := range osTypes.Values() {
		versions = append(versions, semversion.Binary{
			Number:  agentVersion,
			Arch:    arch.HostArch(),
			Release: osType,
		})
		if arch.HostArch() != "amd64" {
			versions = append(versions, semversion.Binary{
				Number:  agentVersion,
				Arch:    "amd64",
				Release: osType,
			})

		}
	}
	return versions
}

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.AuditingEnabled: true,
	}
	s.ControllerModelConfigAttrs = map[string]interface{}{
		"agent-version": coretesting.CurrentVersion().Number.String(),
	}
	s.WithLeaseManager = true
	s.commonMachineSuite.SetUpTest(c)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	// Upload tools to both release and devel streams since config will dictate that we
	// end up looking in both places.
	versions := defaultVersions(coretesting.CurrentVersion().Number)
	envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", versions...)
	envtesting.AssertUploadFakeToolsVersions(c, stor, "devel", "devel", versions...)
	s.agentStorage = stor

	// Restart failed workers much faster for the tests.
	s.PatchValue(&engine.EngineErrorDelay, 100*time.Millisecond)

	// Most of these tests normally finish sub-second on a fast machine.
	// If any given test hits a minute, we have almost certainly become
	// wedged, so dump the logs.
	coretesting.DumpTestLogsAfter(time.Minute, c, s)

	// Ensure the dummy provider is initialised - no need to actually bootstrap.
	ctx := envtesting.BootstrapContext(context.Background(), c)
	err = s.Environ.PrepareForBootstrap(ctx, "controller")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestParseNonsense(c *gc.C) {
	aCfg := agentconf.NewAgentConf(s.DataDir)
	err := ParseAgentCommand(&machineAgentCommand{agentInitializer: aCfg}, nil)
	c.Assert(err, gc.ErrorMatches, "either machine-id or controller-id must be set")
	err = ParseAgentCommand(&machineAgentCommand{agentInitializer: aCfg}, []string{"--machine-id", "-4004"})
	c.Assert(err, gc.ErrorMatches, "--machine-id option must be a non-negative integer")
	err = ParseAgentCommand(&machineAgentCommand{agentInitializer: aCfg}, []string{"--controller-id", "-4004"})
	c.Assert(err, gc.ErrorMatches, "--controller-id option must be a non-negative integer")
}

func (s *MachineSuite) TestParseUnknown(c *gc.C) {
	aCfg := agentconf.NewAgentConf(s.DataDir)
	a := &machineAgentCommand{agentInitializer: aCfg}
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blistering barnacles"\]`)
}

func (s *MachineSuite) TestParseSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	create := func() (cmd.Command, agentconf.AgentConf) {
		aCfg := agentconf.NewAgentConf(s.DataDir)
		s.PrimeAgent(c, names.NewMachineTag("42"), initialMachinePassword)

		newDBWorkerFunc := func(context.Context, dbaccessor.DBApp, string, ...dbaccessor.TrackedDBWorkerOption) (dbaccessor.TrackedDB, error) {
			return databasetesting.NewTrackedDB(s.TxnRunnerFactory()), nil
		}
		a := NewMachineAgentCommand(
			nil,
			NewTestMachineAgentFactory(c, aCfg, newDBWorkerFunc, c.MkDir(), s.cmdRunner),
			aCfg,
			aCfg,
		)
		return a, aCfg
	}
	a := CheckAgentCommand(c, s.DataDir, create, []string{"--machine-id", "42", "--log-to-stderr", "--data-dir", s.DataDir})
	c.Assert(a.(*machineAgentCommand).machineId, gc.Equals, "42")
}

func (s *MachineSuite) TestUseLumberjack(c *gc.C) {
	ctx := cmdtesting.Context(c)
	agentConf := FakeAgentConfig{}

	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	newDBWorkerFunc := func(context.Context, dbaccessor.DBApp, string, ...dbaccessor.TrackedDBWorkerOption) (dbaccessor.TrackedDB, error) {
		return databasetesting.NewTrackedDB(s.TxnRunnerFactory()), nil
	}
	a := NewMachineAgentCommand(
		ctx,
		NewTestMachineAgentFactory(c, &agentConf, newDBWorkerFunc, c.MkDir(), s.cmdRunner),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCommand).machineId = "42"

	err := a.Init(nil)
	c.Assert(err, gc.IsNil)

	l, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsTrue)
	c.Check(l.MaxAge, gc.Equals, 0)
	c.Check(l.MaxBackups, gc.Equals, 2)
	c.Check(l.Filename, gc.Equals, filepath.FromSlash("/var/log/juju/machine-42.log"))
	c.Check(l.MaxSize, gc.Equals, 100)
}

func (s *MachineSuite) TestDontUseLumberjack(c *gc.C) {
	ctx := cmdtesting.Context(c)
	agentConf := FakeAgentConfig{}

	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	newDBWorkerFunc := func(context.Context, dbaccessor.DBApp, string, ...dbaccessor.TrackedDBWorkerOption) (dbaccessor.TrackedDB, error) {
		return databasetesting.NewTrackedDB(s.TxnRunnerFactory()), nil
	}
	a := NewMachineAgentCommand(
		ctx,
		NewTestMachineAgentFactory(c, &agentConf, newDBWorkerFunc, c.MkDir(), s.cmdRunner),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCommand).machineId = "42"

	// set the value that normally gets set by the flag parsing
	a.(*machineAgentCommand).logToStdErr = true

	err := a.Init(nil)
	c.Assert(err, gc.IsNil)

	_, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsFalse)
}

func (s *MachineSuite) TestRunStop(c *gc.C) {
	c.Skip("This test needs to be migrated once we have switched over to dqlite.")

	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	err := a.Stop()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(<-done, jc.ErrorIsNil)
}

func (s *MachineSuite) testUpgradeRequest(c *gc.C, agent runner, tag string, currentTools *tools.Tools, upgrader state.Upgrader) {
	newVers := coretesting.CurrentVersion()
	newVers.Patch++
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.agentStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), newVers)[0]

	// Set a new agent version
	// Currently we have to dual-write, because the actual agent version is
	// read from dqlite, but the check of "should I upgrade" is still being
	// read from Mongo.
	err := s.ControllerModel(c).State().SetModelAgentVersion(newVers.Number, nil, true, upgrader)
	c.Assert(err, jc.ErrorIsNil)
	s.setAgentVersion(c, newVers.Number.String())
	c.Assert(err, jc.ErrorIsNil)

	err = runWithTimeout(c, agent)
	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: tag,
		OldTools:  currentTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir,
	})
}

// setAgentVersion sets the agent version for the controller model in dqlite.
func (s *MachineSuite) setAgentVersion(c *gc.C, vers string) {
	db := s.ModelTxnRunner(c, s.ControllerModelUUID())

	q := "INSERT INTO agent_version (target_version) values ($M.target_version)"

	args := sqlair.M{"target_version": vers}

	stmt, err := sqlair.Prepare(q, args)
	c.Assert(err, jc.ErrorIsNil)

	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, args).Run()
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestUpgradeRequest(c *gc.C) {
	c.Skip("SetModelAgentVersion needs to be implemented a domain.")
	m, _, currentTools := s.primeAgent(c, state.JobManageModel, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	s.testUpgradeRequest(c, a, m.Tag().String(), currentTools, stubUpgrader{})
	c.Assert(a.initialUpgradeCheckComplete.IsUnlocked(), jc.IsFalse)
}

func (s *MachineSuite) TestNoUpgradeRequired(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobManageModel, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	done := make(chan error)
	go func() { done <- a.Run(cmdtesting.Context(c)) }()
	select {
	case <-a.initialUpgradeCheckComplete.Unlocked():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout waiting for upgrade check")
	}
	defer a.Stop() // in case of failure
	s.waitStopped(c, state.JobManageModel, a, done)
	c.Assert(a.initialUpgradeCheckComplete.IsUnlocked(), jc.IsTrue)
}

func (s *MachineSuite) TestAgentSetsToolsVersionManageModel(c *gc.C) {
	c.Skip("This test needs to be migrated once we have switched over to dqlite.")

	s.assertAgentSetsToolsVersion(c, state.JobManageModel)
}

func (s *MachineSuite) TestAgentSetsToolsVersionHostUnits(c *gc.C) {
	c.Skip("This test needs to be migrated once we have switched over to dqlite.")

	s.assertAgentSetsToolsVersion(c, state.JobHostUnits)
}

func (s *MachineSuite) TestMachineAgentRunsAPIAddressUpdaterWorker(c *gc.C) {
	c.Skip("This test needs to be migrated once we have switched over to dqlite.")

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Update the API addresses.
	updatedServers := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "localhost"),
	}

	controllerConfig := coretesting.FakeControllerConfig()

	st := s.ControllerModel(c).State()
	err := st.SetAPIHostPorts(controllerConfig, updatedServers, updatedServers)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for config to be updated.
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if !attempt.HasNext() {
			break
		}
		addrs, err := a.CurrentConfig().APIAddresses()
		c.Assert(err, jc.ErrorIsNil)
		if reflect.DeepEqual(addrs, []string{"localhost:1234"}) {
			return
		}
	}
	c.Fatalf("timeout while waiting for agent config to change")
}

func (s *MachineSuite) TestMachineAgentRunsDiskManagerWorker(c *gc.C) {
	c.Skip("This test needs to be migrated once we have switched over to dqlite.")

	// Patch out the worker func before starting the agent.
	started := newSignal()
	newWorker := func(diskmanager.ListBlockDevicesFunc, diskmanager.BlockDeviceSetter) worker.Worker {
		started.trigger()
		return jworker.NoopWorker()
	}
	s.PatchValue(&diskmanager.NewWorker, newWorker)

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	started.assertTriggered(c, "diskmanager worker to start")
}

func (s *MachineSuite) TestDiskManagerWorkerUpdatesState(c *gc.C) {
	// TODO(wallyworld) - we need the dqlite model database to be available.
	c.Skip("we need to seed the dqlite database with machine data")
	expected := []blockdevice.BlockDevice{{DeviceName: "whatever"}}
	s.PatchValue(&diskmanager.DefaultListBlockDevices, func(context.Context) ([]blockdevice.BlockDevice, error) {
		return expected, nil
	})

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for state to be updated.
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		devices, err := blockdevicestate.NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), m.Id())
		c.Assert(err, jc.ErrorIsNil)
		if len(devices) > 0 {
			c.Assert(devices, gc.HasLen, 1)
			c.Assert(devices[0].DeviceName, gc.Equals, expected[0].DeviceName)
			return
		}
	}
	c.Fatalf("timeout while waiting for block devices to be recorded")
}

func (s *MachineSuite) TestMachineAgentRunsMachineStorageWorker(c *gc.C) {
	c.Skip("This test needs to be migrated once we have switched over to dqlite.")

	m, _, _ := s.primeAgent(c, state.JobHostUnits)

	started := newSignal()
	newWorker := func(config storageprovisioner.Config) (worker.Worker, error) {
		c.Check(config.Scope, gc.Equals, m.Tag())
		c.Check(config.Validate(), jc.ErrorIsNil)
		started.trigger()
		return jworker.NoopWorker(), nil
	}
	s.PatchValue(&storageprovisioner.NewStorageProvisioner, newWorker)

	// Start the machine agent.
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	started.assertTriggered(c, "storage worker to start")
}

func (s *MachineSuite) TestCertificateDNSUpdated(c *gc.C) {
	c.Skip("this should be an integration test and a unit test not this flaky thing")
	m, _, _ := s.primeAgent(c, state.JobManageModel)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	s.testCertificateDNSUpdated(c, a)
}

func (s *MachineSuite) TestCertificateDNSUpdatedInvalidPrivateKey(c *gc.C) {
	c.Skip("this should be an integration test and a unit test not this flaky thing")
	m, agentConfig, _ := s.primeAgent(c, state.JobManageModel)

	// Write out config with an invalid private key. This should
	// cause the agent to rewrite the cert and key.
	si, ok := agentConfig.StateServingInfo()
	c.Assert(ok, jc.IsTrue)
	si.PrivateKey = "foo"
	agentConfig.SetStateServingInfo(si)
	err := agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)

	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	s.testCertificateDNSUpdated(c, a)
}

func (s *MachineSuite) testCertificateDNSUpdated(c *gc.C, a *MachineAgent) {
	// Set up a channel which fires when State is opened.
	started := make(chan struct{}, 16)
	s.PatchValue(&reportOpenedState, func(*state.State) {
		started <- struct{}{}
	})

	// Start the agent.
	go func() { c.Check(a.Run(cmdtesting.Context(c)), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for State to be opened. Once this occurs we know that the
	// agent's initial startup has happened.
	s.assertChannelActive(c, started, "agent to start up")

	// Check that certificate was updated when the agent started.
	stateInfo, _ := a.CurrentConfig().StateServingInfo()
	srvCert, _, err := cert.ParseCertAndKey(stateInfo.Cert, stateInfo.PrivateKey)
	c.Assert(err, jc.ErrorIsNil)
	expectedDnsNames := set.NewStrings("localhost", "juju-apiserver", "juju-mongodb")
	certDnsNames := set.NewStrings(srvCert.DNSNames...)
	c.Check(expectedDnsNames.Difference(certDnsNames).IsEmpty(), jc.IsTrue)

	// Check the mongo certificate file too.
	pemContent, err := os.ReadFile(filepath.Join(s.DataDir, "server.pem"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(pemContent), gc.Equals, stateInfo.Cert+"\n"+stateInfo.PrivateKey)
}

func (s *MachineSuite) setupIgnoreAddresses(c *gc.C, expectedIgnoreValue bool) chan bool {
	ignoreAddressCh := make(chan bool, 1)
	s.AgentSuite.PatchValue(&machiner.NewMachiner, func(cfg machiner.Config) (worker.Worker, error) {
		select {
		case ignoreAddressCh <- cfg.ClearMachineAddressesOnStart:
		default:
		}

		// The test just cares that NewMachiner is called with the correct
		// value, nothing else is done with the worker.
		return jworker.NoopWorker(), nil
	})

	attrs := coretesting.Attrs{"ignore-machine-addresses": expectedIgnoreValue}
	err := s.ControllerDomainServices(c).Config().UpdateModelConfig(context.Background(), attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	return ignoreAddressCh
}

func (s *MachineSuite) TestMachineAgentIgnoreAddresses(c *gc.C) {
	c.Skip("This test needs to be migrated once we have switched over to dqlite.")

	for _, expectedIgnoreValue := range []bool{true, false} {
		ignoreAddressCh := s.setupIgnoreAddresses(c, expectedIgnoreValue)

		m, _, _ := s.primeAgent(c, state.JobHostUnits)
		ctrl, a := s.newAgent(c, m)
		defer ctrl.Finish()
		defer a.Stop()
		doneCh := make(chan error)
		go func() {
			doneCh <- a.Run(nil)
		}()

		select {
		case ignoreMachineAddresses := <-ignoreAddressCh:
			if ignoreMachineAddresses != expectedIgnoreValue {
				c.Fatalf("expected ignore-machine-addresses = %v, got = %v", expectedIgnoreValue, ignoreMachineAddresses)
			}
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for the machiner to start")
		}
		s.waitStopped(c, state.JobHostUnits, a, doneCh)
	}
}

func (s *MachineSuite) TestMachineAgentIgnoreAddressesContainer(c *gc.C) {
	c.Skip("This test needs to be migrated once we have switched over to dqlite.")

	ignoreAddressCh := s.setupIgnoreAddresses(c, true)

	st := s.ControllerModel(c).State()
	parent, err := st.AddMachine(state.UbuntuBase("20.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m, err := st.AddMachineInsideMachine(
		state.MachineTemplate{
			Base: state.UbuntuBase("22.04"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		},
		parent.Id(),
		instance.LXD,
	)
	c.Assert(err, jc.ErrorIsNil)

	vers := coretesting.CurrentVersion()
	s.primeAgentWithMachine(c, m, vers)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	defer a.Stop()
	doneCh := make(chan error)
	go func() {
		doneCh <- a.Run(nil)
	}()

	select {
	case ignoreMachineAddresses := <-ignoreAddressCh:
		if ignoreMachineAddresses {
			c.Fatalf("expected ignore-machine-addresses = false, got = true")
		}
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for the machiner to start")
	}
	s.waitStopped(c, state.JobHostUnits, a, doneCh)
}

func (s *MachineSuite) TestMachineWorkers(c *gc.C) {
	// TODO(wallyworld) - we need the dqlite model database to be available.
	c.Skip("we need to seed the dqlite database with machine data")
	testing.PatchExecutableAsEchoArgs(c, s, "ovs-vsctl", 0)

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackMachines(c, tracker, iaasMachineManifolds)
	s.PatchValue(&iaasMachineManifolds, instrumented)

	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(cmdtesting.Context(c)), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for it to stabilise, running as normal.
	matcher := agenttest.NewWorkerMatcher(c, tracker, a.Tag().String(),
		append(alwaysMachineWorkers, notMigratingMachineWorkers...))

	agenttest.WaitMatch(c, matcher.Check, coretesting.LongWait)
}

func (s *MachineSuite) TestReplicasetInitForNewController(c *gc.C) {
	c.Skip("This test needs to be migrated once we have switched over to dqlite.")

	m, _, _ := s.primeAgent(c, state.JobManageModel)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()

	agentConfig := a.CurrentConfig()

	err := a.ensureMongoServer(context.Background(), agentConfig)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeEnsureMongo.EnsureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.InitiateCount, gc.Equals, 0)
}

func (s *MachineSuite) waitStopped(c *gc.C, job state.MachineJob, a *MachineAgent, done chan error) {
	err := a.Stop()
	if job == state.JobManageModel {
		// When shutting down, the API server can be shut down before
		// the other workers that connect to it, so they get an error so
		// they then die, causing Stop to return an error.  It's not
		// easy to control the actual error that's received in this
		// circumstance so we just log it rather than asserting that it
		// is not nil.
		if err != nil {
			c.Logf("error shutting down state manager: %v", err)
		}
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineSuite) assertAgentSetsToolsVersion(c *gc.C, job state.MachineJob) {
	s.PatchValue(&mongo.IsMaster, func(session *mgo.Session, obj mongo.WithAddresses) (bool, error) {
		addr := obj.Addresses()
		for _, a := range addr {
			if a.Value == "0.1.2.3" {
				return true, nil
			}
		}
		return false, nil
	})
	vers := coretesting.CurrentVersion()
	vers.Minor--
	m, _, _ := s.primeAgentVersion(c, vers, job)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	ctx := cmdtesting.Context(c)
	go func() { c.Check(a.Run(ctx), jc.ErrorIsNil) }()
	defer func() {
		logger.Infof(context.TODO(), "stopping machine agent")
		c.Check(a.Stop(), jc.ErrorIsNil)
		logger.Infof(context.TODO(), "stopped machine agent")
	}()

	timeout := time.After(coretesting.LongWait)
	for done := false; !done; {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for agent version to be set")
		case <-time.After(coretesting.ShortWait):
			c.Log("Refreshing")
			err := m.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			c.Log("Fetching agent tools")
			agentTools, err := m.AgentTools()
			c.Assert(err, jc.ErrorIsNil)
			c.Logf("(%v vs. %v)", agentTools.Version, jujuversion.Current)
			if agentTools.Version.Minor != jujuversion.Current.Minor {
				continue
			}
			c.Assert(agentTools.Version.Number, gc.DeepEquals, jujuversion.Current)
			done = true
		}
	}
}

type stubUpgrader struct {
	upgrading bool
}

func (s stubUpgrader) IsUpgrading() (bool, error) {
	return s.upgrading, nil
}
