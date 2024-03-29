// Code generated by MockGen. DO NOT EDIT.
// Source: discoPeerEndpoint.go

// Package mock_disco is a generated GoMock package.
package mock_disco

import (
	netip "net/netip"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	disco "github.com/miscord-dev/tetrapod/disco"
	ticker "github.com/miscord-dev/tetrapod/disco/ticker"
)

// MockDiscoPeerEndpoint is a mock of DiscoPeerEndpoint interface.
type MockDiscoPeerEndpoint struct {
	ctrl     *gomock.Controller
	recorder *MockDiscoPeerEndpointMockRecorder
}

// MockDiscoPeerEndpointMockRecorder is the mock recorder for MockDiscoPeerEndpoint.
type MockDiscoPeerEndpointMockRecorder struct {
	mock *MockDiscoPeerEndpoint
}

// NewMockDiscoPeerEndpoint creates a new mock instance.
func NewMockDiscoPeerEndpoint(ctrl *gomock.Controller) *MockDiscoPeerEndpoint {
	mock := &MockDiscoPeerEndpoint{ctrl: ctrl}
	mock.recorder = &MockDiscoPeerEndpointMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDiscoPeerEndpoint) EXPECT() *MockDiscoPeerEndpointMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockDiscoPeerEndpoint) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockDiscoPeerEndpointMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockDiscoPeerEndpoint)(nil).Close))
}

// Endpoint mocks base method.
func (m *MockDiscoPeerEndpoint) Endpoint() netip.AddrPort {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Endpoint")
	ret0, _ := ret[0].(netip.AddrPort)
	return ret0
}

// Endpoint indicates an expected call of Endpoint.
func (mr *MockDiscoPeerEndpointMockRecorder) Endpoint() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Endpoint", reflect.TypeOf((*MockDiscoPeerEndpoint)(nil).Endpoint))
}

// EnqueueReceivedPacket mocks base method.
func (m *MockDiscoPeerEndpoint) EnqueueReceivedPacket(pkt disco.DiscoPacket) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "EnqueueReceivedPacket", pkt)
}

// EnqueueReceivedPacket indicates an expected call of EnqueueReceivedPacket.
func (mr *MockDiscoPeerEndpointMockRecorder) EnqueueReceivedPacket(pkt interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnqueueReceivedPacket", reflect.TypeOf((*MockDiscoPeerEndpoint)(nil).EnqueueReceivedPacket), pkt)
}

// ReceivePing mocks base method.
func (m *MockDiscoPeerEndpoint) ReceivePing() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReceivePing")
}

// ReceivePing indicates an expected call of ReceivePing.
func (mr *MockDiscoPeerEndpointMockRecorder) ReceivePing() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReceivePing", reflect.TypeOf((*MockDiscoPeerEndpoint)(nil).ReceivePing))
}

// SetPriority mocks base method.
func (m *MockDiscoPeerEndpoint) SetPriority(priority ticker.Priority) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetPriority", priority)
}

// SetPriority indicates an expected call of SetPriority.
func (mr *MockDiscoPeerEndpointMockRecorder) SetPriority(priority interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetPriority", reflect.TypeOf((*MockDiscoPeerEndpoint)(nil).SetPriority), priority)
}

// Status mocks base method.
func (m *MockDiscoPeerEndpoint) Status() disco.DiscoPeerEndpointStatus {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Status")
	ret0, _ := ret[0].(disco.DiscoPeerEndpointStatus)
	return ret0
}

// Status indicates an expected call of Status.
func (mr *MockDiscoPeerEndpointMockRecorder) Status() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Status", reflect.TypeOf((*MockDiscoPeerEndpoint)(nil).Status))
}

// MockDiscoPeerEndpointStatus is a mock of DiscoPeerEndpointStatus interface.
type MockDiscoPeerEndpointStatus struct {
	ctrl     *gomock.Controller
	recorder *MockDiscoPeerEndpointStatusMockRecorder
}

// MockDiscoPeerEndpointStatusMockRecorder is the mock recorder for MockDiscoPeerEndpointStatus.
type MockDiscoPeerEndpointStatusMockRecorder struct {
	mock *MockDiscoPeerEndpointStatus
}

// NewMockDiscoPeerEndpointStatus creates a new mock instance.
func NewMockDiscoPeerEndpointStatus(ctrl *gomock.Controller) *MockDiscoPeerEndpointStatus {
	mock := &MockDiscoPeerEndpointStatus{ctrl: ctrl}
	mock.recorder = &MockDiscoPeerEndpointStatusMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDiscoPeerEndpointStatus) EXPECT() *MockDiscoPeerEndpointStatusMockRecorder {
	return m.recorder
}

// NotifyStatus mocks base method.
func (m *MockDiscoPeerEndpointStatus) NotifyStatus(fn func(disco.DiscoPeerEndpointStatusReadOnly)) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "NotifyStatus", fn)
}

// NotifyStatus indicates an expected call of NotifyStatus.
func (mr *MockDiscoPeerEndpointStatusMockRecorder) NotifyStatus(fn interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NotifyStatus", reflect.TypeOf((*MockDiscoPeerEndpointStatus)(nil).NotifyStatus), fn)
}
