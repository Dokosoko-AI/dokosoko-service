import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MarkdownMessage } from "../app/components/MarkdownMessage";

test("renders useful assistant Markdown without executable HTML or remote images", () => {
  const html = renderToStaticMarkup(React.createElement(MarkdownMessage, null, [
    "Available API:",
    "",
    "- **ComplicatedAuth Customer API**",
    "- Supported operation: `platform.readiness.check`",
    "",
    "[Documentation](https://docs.example.test) [unsafe](javascript:alert(1))",
    "",
    "<script>alert(1)</script>",
    "",
    "![tracker](https://tracker.invalid/pixel.png)",
  ].join("\n")));

  assert.match(html, /<ul>/);
  assert.match(html, /<strong>ComplicatedAuth Customer API<\/strong>/);
  assert.match(html, /<code>platform\.readiness\.check<\/code>/);
  assert.match(html, /href="https:\/\/docs\.example\.test"[^>]*target="_blank"[^>]*rel="noopener noreferrer"/);
  assert.doesNotMatch(html, /javascript:|<script|<img|tracker\.invalid|alert\(1\)/i);
});
