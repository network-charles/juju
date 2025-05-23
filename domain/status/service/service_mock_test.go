// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/domain/status/service (interfaces: StatusHistory,StatusHistoryReader)
//
// Generated by this command:
//
//	mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/status/service StatusHistory,StatusHistoryReader
//

// Package service is a generated GoMock package.
package service

import (
	context "context"
	reflect "reflect"

	status "github.com/juju/juju/core/status"
	statushistory "github.com/juju/juju/internal/statushistory"
	gomock "go.uber.org/mock/gomock"
)

// MockStatusHistory is a mock of StatusHistory interface.
type MockStatusHistory struct {
	ctrl     *gomock.Controller
	recorder *MockStatusHistoryMockRecorder
}

// MockStatusHistoryMockRecorder is the mock recorder for MockStatusHistory.
type MockStatusHistoryMockRecorder struct {
	mock *MockStatusHistory
}

// NewMockStatusHistory creates a new mock instance.
func NewMockStatusHistory(ctrl *gomock.Controller) *MockStatusHistory {
	mock := &MockStatusHistory{ctrl: ctrl}
	mock.recorder = &MockStatusHistoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStatusHistory) EXPECT() *MockStatusHistoryMockRecorder {
	return m.recorder
}

// RecordStatus mocks base method.
func (m *MockStatusHistory) RecordStatus(arg0 context.Context, arg1 statushistory.Namespace, arg2 status.StatusInfo) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RecordStatus", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// RecordStatus indicates an expected call of RecordStatus.
func (mr *MockStatusHistoryMockRecorder) RecordStatus(arg0, arg1, arg2 any) *MockStatusHistoryRecordStatusCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RecordStatus", reflect.TypeOf((*MockStatusHistory)(nil).RecordStatus), arg0, arg1, arg2)
	return &MockStatusHistoryRecordStatusCall{Call: call}
}

// MockStatusHistoryRecordStatusCall wrap *gomock.Call
type MockStatusHistoryRecordStatusCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStatusHistoryRecordStatusCall) Return(arg0 error) *MockStatusHistoryRecordStatusCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStatusHistoryRecordStatusCall) Do(f func(context.Context, statushistory.Namespace, status.StatusInfo) error) *MockStatusHistoryRecordStatusCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStatusHistoryRecordStatusCall) DoAndReturn(f func(context.Context, statushistory.Namespace, status.StatusInfo) error) *MockStatusHistoryRecordStatusCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// MockStatusHistoryReader is a mock of StatusHistoryReader interface.
type MockStatusHistoryReader struct {
	ctrl     *gomock.Controller
	recorder *MockStatusHistoryReaderMockRecorder
}

// MockStatusHistoryReaderMockRecorder is the mock recorder for MockStatusHistoryReader.
type MockStatusHistoryReaderMockRecorder struct {
	mock *MockStatusHistoryReader
}

// NewMockStatusHistoryReader creates a new mock instance.
func NewMockStatusHistoryReader(ctrl *gomock.Controller) *MockStatusHistoryReader {
	mock := &MockStatusHistoryReader{ctrl: ctrl}
	mock.recorder = &MockStatusHistoryReaderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStatusHistoryReader) EXPECT() *MockStatusHistoryReaderMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockStatusHistoryReader) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockStatusHistoryReaderMockRecorder) Close() *MockStatusHistoryReaderCloseCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockStatusHistoryReader)(nil).Close))
	return &MockStatusHistoryReaderCloseCall{Call: call}
}

// MockStatusHistoryReaderCloseCall wrap *gomock.Call
type MockStatusHistoryReaderCloseCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStatusHistoryReaderCloseCall) Return(arg0 error) *MockStatusHistoryReaderCloseCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStatusHistoryReaderCloseCall) Do(f func() error) *MockStatusHistoryReaderCloseCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStatusHistoryReaderCloseCall) DoAndReturn(f func() error) *MockStatusHistoryReaderCloseCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Walk mocks base method.
func (m *MockStatusHistoryReader) Walk(arg0 func(statushistory.HistoryRecord) (bool, error)) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Walk", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Walk indicates an expected call of Walk.
func (mr *MockStatusHistoryReaderMockRecorder) Walk(arg0 any) *MockStatusHistoryReaderWalkCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Walk", reflect.TypeOf((*MockStatusHistoryReader)(nil).Walk), arg0)
	return &MockStatusHistoryReaderWalkCall{Call: call}
}

// MockStatusHistoryReaderWalkCall wrap *gomock.Call
type MockStatusHistoryReaderWalkCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStatusHistoryReaderWalkCall) Return(arg0 error) *MockStatusHistoryReaderWalkCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStatusHistoryReaderWalkCall) Do(f func(func(statushistory.HistoryRecord) (bool, error)) error) *MockStatusHistoryReaderWalkCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStatusHistoryReaderWalkCall) DoAndReturn(f func(func(statushistory.HistoryRecord) (bool, error)) error) *MockStatusHistoryReaderWalkCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}
