import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

function safeMarkdownURL(url: string) {
  try {
    const protocol = new URL(url).protocol;
    return protocol === "https:" || protocol === "http:" || protocol === "mailto:" ? url : "";
  } catch {
    return "";
  }
}

export function MarkdownMessage({ children }: { children: string }) {
  return <div className="markdown-message">
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      skipHtml
      urlTransform={safeMarkdownURL}
      components={{
        a: ({ href, title, children: linkChildren }) => href
          ? <a href={href} title={title} target="_blank" rel="noopener noreferrer">{linkChildren}</a>
          : <>{linkChildren}</>,
        img: ({ alt }) => alt ? <em className="markdown-image-alt">{alt}</em> : null,
      }}
    >
      {children}
    </ReactMarkdown>
  </div>;
}
