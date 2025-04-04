// Code generated by mockery v2.53.3. DO NOT EDIT.

package mocks

import (
	repository "backend/internal/repository"

	mock "github.com/stretchr/testify/mock"
)

// StatisticGetter is an autogenerated mock type for the StatisticGetter type
type StatisticGetter struct {
	mock.Mock
}

// GetStatistic provides a mock function with given fields: shortID
func (_m *StatisticGetter) GetStatistic(shortID string) (*repository.StatisticResponse, error) {
	ret := _m.Called(shortID)

	if len(ret) == 0 {
		panic("no return value specified for GetStatistic")
	}

	var r0 *repository.StatisticResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (*repository.StatisticResponse, error)); ok {
		return rf(shortID)
	}
	if rf, ok := ret.Get(0).(func(string) *repository.StatisticResponse); ok {
		r0 = rf(shortID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*repository.StatisticResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(shortID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewStatisticGetter creates a new instance of StatisticGetter. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewStatisticGetter(t interface {
	mock.TestingT
	Cleanup(func())
}) *StatisticGetter {
	mock := &StatisticGetter{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
