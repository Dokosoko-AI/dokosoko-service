# Native tool plugin example

This source package demonstrates two tools:

- `example.echo` accepts anonymous or authenticated calls and receives only an opaque actor ID when one exists.
- `example.increment_counter` requires a customer identity, receives customer-scoped state, and records each idempotency key transactionally.

It is deliberately not active by default. Review the package, then import it in `internal/nativeplugins/registry.go` and add `sampleplugin.New()` to `Registered`. DokoSoko discovers it at startup and creates reviewable draft Tool rows. Configure the optional prefix with `DOKOSOKO_PLUGIN_EXAMPLE_COUNTER_GREETING_PREFIX`.

Read the complete [native plugin contract](../../docs/NATIVE_TOOL_PLUGINS.md)
before adapting this package. The source checker is a review aid, not a
sandbox; review the complete dependency graph before compiling a plugin into a
production service.

Run its conformance tests and strict source check from the service root:

```sh
go test ./examples/native-tool-plugin
go run ./cmd/dokosoko-native-plugin-check ./examples/native-tool-plugin
```
