---
paths:
  - pkg/*/*.go
  - pkg/*/*/*.go
  - pkg/*/*/*/*.go
  - pkg/*/*/*/*/*.go
---
A package enters toolkit-go only when a public tool repository imports
it, and only in unbranded, generalised form: neutral names, no
package-level mutable state (configuration is a value, the environment
is injected through `getenv`), unit tests in the package, and no
fixtures captured from real sessions. Interfaces stay small, one to
five methods, with constructors that wire the common case, so a tool
composes the capabilities it needs and inherits nothing else. No file
names a private repository, private tooling or an agent-context path.
