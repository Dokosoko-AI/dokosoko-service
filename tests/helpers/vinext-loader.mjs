// Match the production vinext alias when rendering owned controls in Node tests.
export function resolve(specifier, context, nextResolve) {
  return nextResolve(specifier === "next/link" ? "vinext/shims/link" : specifier, context);
}
