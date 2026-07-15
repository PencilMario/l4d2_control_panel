# Task Intent

Implement the approved per-instance SRCDS startup configuration and plugin package design. Creation and editing must share one configuration contract, render a live command preview, preserve one selected and one applied package identity, install and deploy content before first SRCDS start, and serialize reconfiguration through the existing persistent Job owner.

In scope: additive SQLite state, API validation and reconfiguration, canonical argv parsing, Docker maintenance installation, lifecycle provisioning, package-state persistence, React configuration UI, runtime enforcement, unit/integration/browser verification.

Out of scope: multiple packages per instance, arbitrary shell or Docker commands, plugin repository redesign, package deletion, schedule model redesign, remote agents and RBAC.
