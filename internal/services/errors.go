package services

import (
	"fmt"
)

// ServiceError is the base interface for all service-related errors
type ServiceError interface {
	error
	ServiceName() string
}

// ServiceNotFoundError indicates a service was not found in launchd
type ServiceNotFoundError struct {
	Name string
}

func (e ServiceNotFoundError) Error() string {
	return fmt.Sprintf("service not found: %s", e.Name)
}

func (e ServiceNotFoundError) ServiceName() string {
	return e.Name
}

// PlistNotFoundError indicates a plist file was not found
type PlistNotFoundError struct {
	Name string
	Path string
}

func (e PlistNotFoundError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("plist not found for service %s at path %s", e.Name, e.Path)
	}
	return fmt.Sprintf("plist not found for service: %s", e.Name)
}

func (e PlistNotFoundError) ServiceName() string {
	return e.Name
}

// InvalidPlistError indicates a plist file could not be parsed
type InvalidPlistError struct {
	Name  string
	Path  string
	Cause error
}

func (e InvalidPlistError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("invalid plist for service %s at %s: %v", e.Name, e.Path, e.Cause)
	}
	return fmt.Sprintf("invalid plist for service %s: %v", e.Name, e.Cause)
}

func (e InvalidPlistError) ServiceName() string {
	return e.Name
}

func (e InvalidPlistError) Unwrap() error {
	return e.Cause
}

// LaunchctlError indicates an error running launchctl command
type LaunchctlError struct {
	Command string
	Cause   error
	Output  string
}

func (e LaunchctlError) Error() string {
	if e.Output != "" {
		return fmt.Sprintf("launchctl %s failed: %v (output: %s)", e.Command, e.Cause, e.Output)
	}
	return fmt.Sprintf("launchctl %s failed: %v", e.Command, e.Cause)
}

func (e LaunchctlError) Unwrap() error {
	return e.Cause
}

// UserAgentPathError indicates an error with the user agent directory
type UserAgentPathError struct {
	Path  string
	Cause error
}

func (e UserAgentPathError) Error() string {
	return fmt.Sprintf("user agent path error at %s: %v", e.Path, e.Cause)
}

func (e UserAgentPathError) Unwrap() error {
	return e.Cause
}

// SystemctlError indicates an error running systemctl command
type SystemctlError struct {
	Command string
	Scope   string
	Cause   error
	Output  string
}

func (e SystemctlError) Error() string {
	scopeStr := ""
	if e.Scope != "" {
		scopeStr = fmt.Sprintf(" (%s)", e.Scope)
	}
	if e.Output != "" {
		return fmt.Sprintf("systemctl %s%s failed: %v (output: %s)", e.Command, scopeStr, e.Cause, e.Output)
	}
	return fmt.Sprintf("systemctl %s%s failed: %v", e.Command, scopeStr, e.Cause)
}

func (e SystemctlError) Unwrap() error {
	return e.Cause
}
