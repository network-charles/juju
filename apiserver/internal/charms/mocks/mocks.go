// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/apiserver/internal/charms (interfaces: CharmService,ApplicationService)
//
// Generated by this command:
//
//	mockgen -typed -package mocks -destination mocks/mocks.go github.com/juju/juju/apiserver/internal/charms CharmService,ApplicationService
//

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	application "github.com/juju/juju/core/application"
	charm "github.com/juju/juju/domain/application/charm"
	charm0 "github.com/juju/juju/internal/charm"
	gomock "go.uber.org/mock/gomock"
)

// MockCharmService is a mock of CharmService interface.
type MockCharmService struct {
	ctrl     *gomock.Controller
	recorder *MockCharmServiceMockRecorder
}

// MockCharmServiceMockRecorder is the mock recorder for MockCharmService.
type MockCharmServiceMockRecorder struct {
	mock *MockCharmService
}

// NewMockCharmService creates a new mock instance.
func NewMockCharmService(ctrl *gomock.Controller) *MockCharmService {
	mock := &MockCharmService{ctrl: ctrl}
	mock.recorder = &MockCharmServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCharmService) EXPECT() *MockCharmServiceMockRecorder {
	return m.recorder
}

// GetCharm mocks base method.
func (m *MockCharmService) GetCharm(arg0 context.Context, arg1 charm.CharmLocator) (charm0.Charm, charm.CharmLocator, bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCharm", arg0, arg1)
	ret0, _ := ret[0].(charm0.Charm)
	ret1, _ := ret[1].(charm.CharmLocator)
	ret2, _ := ret[2].(bool)
	ret3, _ := ret[3].(error)
	return ret0, ret1, ret2, ret3
}

// GetCharm indicates an expected call of GetCharm.
func (mr *MockCharmServiceMockRecorder) GetCharm(arg0, arg1 any) *MockCharmServiceGetCharmCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCharm", reflect.TypeOf((*MockCharmService)(nil).GetCharm), arg0, arg1)
	return &MockCharmServiceGetCharmCall{Call: call}
}

// MockCharmServiceGetCharmCall wrap *gomock.Call
type MockCharmServiceGetCharmCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockCharmServiceGetCharmCall) Return(arg0 charm0.Charm, arg1 charm.CharmLocator, arg2 bool, arg3 error) *MockCharmServiceGetCharmCall {
	c.Call = c.Call.Return(arg0, arg1, arg2, arg3)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockCharmServiceGetCharmCall) Do(f func(context.Context, charm.CharmLocator) (charm0.Charm, charm.CharmLocator, bool, error)) *MockCharmServiceGetCharmCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockCharmServiceGetCharmCall) DoAndReturn(f func(context.Context, charm.CharmLocator) (charm0.Charm, charm.CharmLocator, bool, error)) *MockCharmServiceGetCharmCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// MockApplicationService is a mock of ApplicationService interface.
type MockApplicationService struct {
	ctrl     *gomock.Controller
	recorder *MockApplicationServiceMockRecorder
}

// MockApplicationServiceMockRecorder is the mock recorder for MockApplicationService.
type MockApplicationServiceMockRecorder struct {
	mock *MockApplicationService
}

// NewMockApplicationService creates a new mock instance.
func NewMockApplicationService(ctrl *gomock.Controller) *MockApplicationService {
	mock := &MockApplicationService{ctrl: ctrl}
	mock.recorder = &MockApplicationServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockApplicationService) EXPECT() *MockApplicationServiceMockRecorder {
	return m.recorder
}

// GetApplicationIDByName mocks base method.
func (m *MockApplicationService) GetApplicationIDByName(arg0 context.Context, arg1 string) (application.ID, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetApplicationIDByName", arg0, arg1)
	ret0, _ := ret[0].(application.ID)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetApplicationIDByName indicates an expected call of GetApplicationIDByName.
func (mr *MockApplicationServiceMockRecorder) GetApplicationIDByName(arg0, arg1 any) *MockApplicationServiceGetApplicationIDByNameCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetApplicationIDByName", reflect.TypeOf((*MockApplicationService)(nil).GetApplicationIDByName), arg0, arg1)
	return &MockApplicationServiceGetApplicationIDByNameCall{Call: call}
}

// MockApplicationServiceGetApplicationIDByNameCall wrap *gomock.Call
type MockApplicationServiceGetApplicationIDByNameCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockApplicationServiceGetApplicationIDByNameCall) Return(arg0 application.ID, arg1 error) *MockApplicationServiceGetApplicationIDByNameCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockApplicationServiceGetApplicationIDByNameCall) Do(f func(context.Context, string) (application.ID, error)) *MockApplicationServiceGetApplicationIDByNameCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockApplicationServiceGetApplicationIDByNameCall) DoAndReturn(f func(context.Context, string) (application.ID, error)) *MockApplicationServiceGetApplicationIDByNameCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// GetCharmByApplicationID mocks base method.
func (m *MockApplicationService) GetCharmByApplicationID(arg0 context.Context, arg1 application.ID) (charm0.Charm, charm.CharmLocator, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCharmByApplicationID", arg0, arg1)
	ret0, _ := ret[0].(charm0.Charm)
	ret1, _ := ret[1].(charm.CharmLocator)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetCharmByApplicationID indicates an expected call of GetCharmByApplicationID.
func (mr *MockApplicationServiceMockRecorder) GetCharmByApplicationID(arg0, arg1 any) *MockApplicationServiceGetCharmByApplicationIDCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCharmByApplicationID", reflect.TypeOf((*MockApplicationService)(nil).GetCharmByApplicationID), arg0, arg1)
	return &MockApplicationServiceGetCharmByApplicationIDCall{Call: call}
}

// MockApplicationServiceGetCharmByApplicationIDCall wrap *gomock.Call
type MockApplicationServiceGetCharmByApplicationIDCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockApplicationServiceGetCharmByApplicationIDCall) Return(arg0 charm0.Charm, arg1 charm.CharmLocator, arg2 error) *MockApplicationServiceGetCharmByApplicationIDCall {
	c.Call = c.Call.Return(arg0, arg1, arg2)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockApplicationServiceGetCharmByApplicationIDCall) Do(f func(context.Context, application.ID) (charm0.Charm, charm.CharmLocator, error)) *MockApplicationServiceGetCharmByApplicationIDCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockApplicationServiceGetCharmByApplicationIDCall) DoAndReturn(f func(context.Context, application.ID) (charm0.Charm, charm.CharmLocator, error)) *MockApplicationServiceGetCharmByApplicationIDCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}
