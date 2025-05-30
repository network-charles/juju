// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttest

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/controller"
	coredatabase "github.com/juju/juju/core/database"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/mongo/mongotest"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
)

// AgentSuite is a fixture to be used by agent test suites.
type AgentSuite struct {
	testing.ApiServerSuite

	Environ environs.Environ
	DataDir string
	LogDir  string
}

func (s *AgentSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	domainServices := s.ControllerDomainServices(c)

	var err error
	s.Environ, err = stateenvirons.GetNewEnvironFunc(environs.New)(
		s.ControllerModel(c),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Config(),
	)
	c.Assert(err, jc.ErrorIsNil)

	s.DataDir = c.MkDir()
	s.LogDir = c.MkDir()
}

func mongoInfo() *mongo.MongoInfo {
	info := statetesting.NewMongoInfo()
	info.Password = testing.AdminSecret
	return info
}

// PrimeAgent writes the configuration file and tools for an agent
// with the given entity name. It returns the agent's configuration and the
// current tools.
func (s *AgentSuite) PrimeAgent(c *gc.C, tag names.Tag, password string) (agent.ConfigSetterWriter, *coretools.Tools) {
	vers := coretesting.CurrentVersion()
	return s.PrimeAgentVersion(c, tag, password, vers)
}

// PrimeAgentVersion writes the configuration file and tools with version
// vers for an agent with the given entity name. It returns the agent's
// configuration and the current tools.
func (s *AgentSuite) PrimeAgentVersion(c *gc.C, tag names.Tag, password string, vers semversion.Binary) (agent.ConfigSetterWriter, *coretools.Tools) {
	c.Logf("priming agent %s", tag.String())

	store, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)

	agentTools := envtesting.PrimeTools(c, store, s.DataDir, "released", vers)
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	err = envtools.MergeAndWriteMetadata(context.Background(), ss, store, "released", "released", coretools.List{agentTools}, envtools.DoNotWriteMirrors)
	c.Assert(err, jc.ErrorIsNil)

	tools1, err := agenttools.ChangeAgentTools(s.DataDir, tag.String(), vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools1, gc.DeepEquals, agentTools)

	stateInfo := mongoInfo()
	apiInfo := s.ControllerModelApiInfo()

	paths := agent.DefaultPaths
	paths.DataDir = s.DataDir
	paths.TransientDataDir = c.MkDir()
	paths.LogDir = s.LogDir
	paths.MetricsSpoolDir = c.MkDir()

	dqlitePort := mgotesting.FindTCPPort()

	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             paths,
			Tag:               tag,
			UpgradedToVersion: vers.Number,
			Password:          password,
			Nonce:             agent.BootstrapNonce,
			APIAddresses:      apiInfo.Addrs,
			CACert:            stateInfo.CACert,
			Controller:        coretesting.ControllerTag,
			Model:             apiInfo.ModelTag,

			QueryTracingEnabled:   controller.DefaultQueryTracingEnabled,
			QueryTracingThreshold: controller.DefaultQueryTracingThreshold,

			OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
			OpenTelemetryEndpoint:              "",
			OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
			OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
			OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
			OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,

			ObjectStoreType: controller.DefaultObjectStoreType,

			DqlitePort: dqlitePort,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	conf.SetPassword(password)
	c.Assert(conf.Write(), gc.IsNil)

	s.primeAPIHostPorts(c)
	return conf, agentTools
}

// PrimeStateAgentVersion writes the configuration file and tools with
// version vers for a state agent with the given entity name. It
// returns the agent's configuration and the current tools.
func (s *AgentSuite) PrimeStateAgentVersion(c *gc.C, tag names.Tag, password string, vers semversion.Binary) (
	agent.ConfigSetterWriter, *coretools.Tools,
) {
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)

	agentTools := envtesting.PrimeTools(c, stor, s.DataDir, "released", vers)
	tools1, err := agenttools.ChangeAgentTools(s.DataDir, tag.String(), vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools1, gc.DeepEquals, agentTools)

	domainServices := s.ControllerDomainServices(c)
	cfg, err := domainServices.ControllerConfig().ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	apiPort, ok := cfg[controller.APIPort].(int)
	if !ok {
		c.Fatalf("no api port in controller config")
	}
	conf := s.WriteStateAgentConfig(c, tag, password, vers, s.ControllerModel(c).ModelTag(), apiPort)
	s.primeAPIHostPorts(c)

	err = database.BootstrapDqlite(
		context.Background(),
		database.NewNodeManager(conf, true, loggertesting.WrapCheckLog(c), coredatabase.NoopSlowQueryLogger{}),
		modeltesting.GenModelUUID(c),
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	return conf, agentTools
}

// WriteStateAgentConfig creates and writes a state agent config.
func (s *AgentSuite) WriteStateAgentConfig(
	c *gc.C,
	tag names.Tag,
	password string,
	vers semversion.Binary,
	modelTag names.ModelTag,
	apiPort int,
) agent.ConfigSetterWriter {
	stateInfo := mongoInfo()
	apiAddr := []string{fmt.Sprintf("localhost:%d", apiPort)}
	dqlitePort := mgotesting.FindTCPPort()
	conf, err := agent.NewStateMachineConfig(
		agent.AgentConfigParams{
			Paths: agent.NewPathsWithDefaults(agent.Paths{
				DataDir: s.DataDir,
				LogDir:  s.LogDir,
			}),
			Tag:                   tag,
			UpgradedToVersion:     vers.Number,
			Password:              password,
			Nonce:                 agent.BootstrapNonce,
			APIAddresses:          apiAddr,
			CACert:                stateInfo.CACert,
			Controller:            s.ControllerModel(c).ControllerTag(),
			Model:                 modelTag,
			QueryTracingEnabled:   controller.DefaultQueryTracingEnabled,
			QueryTracingThreshold: controller.DefaultQueryTracingThreshold,

			OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
			OpenTelemetryEndpoint:              "",
			OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
			OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
			OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
			OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,

			ObjectStoreType: controller.DefaultObjectStoreType,

			DqlitePort: dqlitePort,
		},
		controller.StateServingInfo{
			Cert:         coretesting.ServerCert,
			PrivateKey:   coretesting.ServerKey,
			CAPrivateKey: coretesting.CAKey,
			StatePort:    mgotesting.MgoServer.Port(),
			APIPort:      apiPort,
		})
	c.Assert(err, jc.ErrorIsNil)

	conf.SetPassword(password)
	c.Assert(conf.Write(), gc.IsNil)

	return conf
}

func (s *AgentSuite) primeAPIHostPorts(c *gc.C) {
	apiInfo := s.ControllerModelApiInfo()

	c.Assert(apiInfo.Addrs, gc.HasLen, 1)
	mHP, err := network.ParseMachineHostPort(apiInfo.Addrs[0])
	c.Assert(err, jc.ErrorIsNil)

	hostPorts := network.SpaceHostPorts{
		{SpaceAddress: network.SpaceAddress{MachineAddress: mHP.MachineAddress}, NetPort: mHP.NetPort}}

	domainServices := s.ControllerDomainServices(c)
	controllerConfig, err := domainServices.ControllerConfig().ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	st := s.ControllerModel(c).State()

	err = st.SetAPIHostPorts(controllerConfig, []network.SpaceHostPorts{hostPorts}, []network.SpaceHostPorts{hostPorts})
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("api host ports primed %#v", hostPorts)
}

// InitAgent initialises the given agent command with additional
// arguments as provided.
func (s *AgentSuite) InitAgent(c *gc.C, a cmd.Command, args ...string) {
	args = append([]string{"--data-dir", s.DataDir}, args...)
	err := cmdtesting.InitCommand(a, args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AgentSuite) AssertCanOpenState(c *gc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, jc.ErrorIsNil)

	info, ok := config.MongoInfo()
	c.Assert(ok, jc.IsTrue)

	session, err := mongo.DialWithInfo(*info, mongotest.DialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      config.Controller(),
		ControllerModelTag: config.Model(),
		MongoSession:       session,
		NewPolicy:          stateenvirons.GetNewPolicyFunc(nil, nil, nil, nil),
	})
	c.Assert(err, jc.ErrorIsNil)
	_ = pool.Close()
}

func (s *AgentSuite) AssertCannotOpenState(c *gc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, jc.ErrorIsNil)

	_, ok := config.MongoInfo()
	c.Assert(ok, jc.IsFalse)
}
