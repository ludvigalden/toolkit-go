// Package supervisor is a small service-supervisor seam: systemd-user on
// Linux, launchd on macOS, and a setsid fallback, chosen by the selector
// subpackage. Backends only touch units they derive from the Spec.
package supervisor
