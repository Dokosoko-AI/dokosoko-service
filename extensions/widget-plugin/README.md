# Widget plugin scaffold

The embedded widget is intentionally not part of `dokosoko-service`. A future
widget should be a separately deployed application that connects through the
same stable product boundaries as any other client:

- private OAuth for user authentication;
- private MCP for reviewed resources and tools;
- an external widget backend for session and browser security;
- an external browser package for presentation.

`dokosoko-service` must not load widget code, store widget sessions or secrets,
or expose widget-specific runtime routes. The example manifest documents the
discovery metadata a future plugin registry could own without coupling that
registry to the core service.

Copy `plugin.example.json` into a separate widget repository, replace the
placeholder URLs, and implement those endpoints there. The plugin should use
standard DokoSoko OAuth and MCP endpoints instead of private in-process APIs.

