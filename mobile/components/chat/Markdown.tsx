import { Fragment } from "react";
import { Image, Linking, Pressable, View } from "react-native";

import { cn } from "@/lib/utils";
import { Text } from "@/components/ui/Text";

// Lightweight markdown renderer for chat. We deliberately do NOT pull
// in react-native-markdown-display — its peer-dep churn (reanimated,
// css-to-react-native) bites every Expo SDK bump, and the agent's
// output uses only a small subset of markdown we can handle in ~250
// lines.
//
// The previous version had a bug where bold (`**…**`) wouldn't
// render with a visibly heavier weight on RN-web: NativeWind's
// `font-semibold` class doesn't always propagate from a nested
// `<Text>` inside a parent `<Text>` in RN-web, so the inner span
// inherited the parent's font-weight. The fix is to apply
// `fontWeight` via inline `style` (which always wins) instead of
// className for inline emphasis. We do the same for inline code, so
// code spans pick up the monospaced family deterministically.
//
// What's intentionally NOT supported: tables, nested lists. Inline
// images inside paragraphs are also not supported — only standalone
// `![alt](url)` lines render as image blocks (which is exactly how
// the chat composer emits them when attaching photos).
//
// Block-level grammar:
//   ```\n…\n```          → fenced code block
//   # / ## / ### Heading → heading block (h1-h3)
//   blank line          → paragraph break
//   - / *               → bulleted list item (single level)
//   1. / 2.             → numbered list item (single level)
//   > quote             → blockquote (single level)
//   ---  / ***          → horizontal rule
//   ![alt](https://…)   → image block (one per line; standalone)
//   anything            → paragraph
//
// Inline grammar (applied within paragraphs and list items):
//   `code`           → monospaced span
//   **bold**         → bold span
//   _italic_         → italic span (also `*italic*` when not bold)
//   [text](url)      → underlined clickable link
//   bare http(s)://… → underlined clickable link

type MarkdownProps = {
  content: string;
  className?: string;
  // Tone for the base paragraph color. The chat surfaces use this so
  // text inside a "user" bubble (dark text on accent bg) renders
  // legibly without forking the whole renderer.
  tone?: "default" | "onAccent";
};

export function Markdown({ content, className, tone = "default" }: MarkdownProps) {
  const blocks = parseBlocks(content);
  return (
    <View className={cn("gap-2", className)}>
      {blocks.map((block, i) => (
        <Block key={i} block={block} tone={tone} />
      ))}
    </View>
  );
}

type Block =
  | { kind: "code"; lang?: string; text: string }
  | { kind: "paragraph"; text: string }
  | { kind: "heading"; level: 1 | 2 | 3; text: string }
  | { kind: "list"; ordered: boolean; items: string[] }
  | { kind: "quote"; text: string }
  | { kind: "rule" }
  | { kind: "image"; alt: string; url: string };

const IMAGE_LINE_RE = /^\s*!\[([^\]]*)\]\((https?:\/\/[^\s)]+)\)\s*$/;
const HEADING_RE = /^\s*(#{1,3})\s+(.*)$/;
const QUOTE_RE = /^\s*>\s?(.*)$/;
const RULE_RE = /^\s*(?:-{3,}|\*{3,}|_{3,})\s*$/;

function parseBlocks(input: string): Block[] {
  const blocks: Block[] = [];
  const lines = input.replace(/\r\n/g, "\n").split("\n");

  let i = 0;
  while (i < lines.length) {
    const line = lines[i];

    // Fenced code block. Match ``` optionally followed by a language tag.
    const fenceMatch = /^```([\w-]*)\s*$/.exec(line);
    if (fenceMatch) {
      const lang = fenceMatch[1] || undefined;
      const buf: string[] = [];
      i++;
      while (i < lines.length && !/^```\s*$/.test(lines[i])) {
        buf.push(lines[i]);
        i++;
      }
      if (i < lines.length) i++;
      blocks.push({ kind: "code", lang, text: buf.join("\n") });
      continue;
    }

    if (line.trim() === "") {
      i++;
      continue;
    }

    // Horizontal rule — `---`, `***`, or `___` on a line by itself.
    if (RULE_RE.test(line)) {
      blocks.push({ kind: "rule" });
      i++;
      continue;
    }

    // Headings. We support h1/h2/h3; lower levels just degrade to h3
    // styling because the chat doesn't have room for a 4-level
    // hierarchy.
    const headingMatch = HEADING_RE.exec(line);
    if (headingMatch) {
      const hashes = headingMatch[1].length;
      const level = (Math.min(hashes, 3) as 1 | 2 | 3);
      blocks.push({ kind: "heading", level, text: headingMatch[2].trim() });
      i++;
      continue;
    }

    // Standalone image line. The chat composer emits one of these per
    // attached photo when sending the user's turn, so it's the
    // expected shape inside the operator's bubble.
    const imageMatch = IMAGE_LINE_RE.exec(line);
    if (imageMatch) {
      blocks.push({ kind: "image", alt: imageMatch[1], url: imageMatch[2] });
      i++;
      continue;
    }

    // Blockquote — a contiguous run of `> ` lines collapses into a
    // single quote block.
    if (QUOTE_RE.test(line)) {
      const buf: string[] = [];
      while (i < lines.length) {
        const m = QUOTE_RE.exec(lines[i]);
        if (!m) break;
        buf.push(m[1]);
        i++;
      }
      blocks.push({ kind: "quote", text: buf.join("\n") });
      continue;
    }

    // List (single level). Collect contiguous list items of the same
    // kind. We also tolerate blank lines BETWEEN items because the
    // agent often emits its numbered lists with blank-line spacing
    // ("loose" list style) — without this look-ahead each item would
    // become its own one-element list and they'd all renumber to "1.".
    const bulletMatch = /^\s*[-*]\s+(.*)$/.exec(line);
    const orderedMatch = /^\s*\d+\.\s+(.*)$/.exec(line);
    if (bulletMatch || orderedMatch) {
      const ordered = !!orderedMatch;
      const items: string[] = [];
      while (i < lines.length) {
        const cur = lines[i];
        const bm = /^\s*[-*]\s+(.*)$/.exec(cur);
        const om = /^\s*\d+\.\s+(.*)$/.exec(cur);
        if (ordered ? om : bm) {
          items.push((ordered ? om![1] : bm![1]).trim());
          i++;
          continue;
        }
        if (cur.trim() === "") {
          // Peek ahead: only treat the blank as part of the list if a
          // matching item resumes on the next non-blank line.
          let j = i + 1;
          while (j < lines.length && lines[j].trim() === "") j++;
          if (j >= lines.length) break;
          const next = lines[j];
          const nextMatches = ordered
            ? /^\s*\d+\.\s+/.test(next)
            : /^\s*[-*]\s+/.test(next);
          if (!nextMatches) break;
          i = j;
          continue;
        }
        break;
      }
      blocks.push({ kind: "list", ordered, items });
      continue;
    }

    // Paragraph: consume until the next block-level boundary.
    const buf: string[] = [];
    while (i < lines.length) {
      const cur = lines[i];
      if (
        cur.trim() === "" ||
        /^```/.test(cur) ||
        /^\s*[-*]\s+/.test(cur) ||
        /^\s*\d+\.\s+/.test(cur) ||
        HEADING_RE.test(cur) ||
        QUOTE_RE.test(cur) ||
        RULE_RE.test(cur) ||
        IMAGE_LINE_RE.test(cur)
      ) {
        break;
      }
      buf.push(cur);
      i++;
    }
    blocks.push({ kind: "paragraph", text: buf.join("\n") });
  }
  return blocks;
}

function Block({ block, tone }: { block: Block; tone: "default" | "onAccent" }) {
  if (block.kind === "code") {
    return (
      <View className="rounded-lg border border-border bg-background/40 px-3 py-2">
        <Text
          className="text-xs text-foreground/90"
          style={{ fontFamily: monoFont(), lineHeight: 18 }}
        >
          {block.text}
        </Text>
      </View>
    );
  }
  if (block.kind === "heading") {
    return <Heading level={block.level} text={block.text} tone={tone} />;
  }
  if (block.kind === "rule") {
    return <View className="my-1 h-px self-stretch bg-border" />;
  }
  if (block.kind === "quote") {
    return (
      <View className="flex-row gap-2">
        <View className="w-0.5 self-stretch rounded-full bg-primary/40" />
        <View className="flex-1 py-0.5">
          <InlineText text={block.text} tone={tone} italic />
        </View>
      </View>
    );
  }
  if (block.kind === "image") {
    return <MarkdownImage url={block.url} alt={block.alt} tone={tone} />;
  }
  if (block.kind === "list") {
    return (
      <View className="gap-1">
        {block.items.map((item, i) => (
          <View key={i} className="flex-row gap-2">
            <Text
              variant="small"
              className={tone === "onAccent" ? "text-background/80" : "text-foreground/80"}
              style={{ minWidth: 14 }}
            >
              {block.ordered ? `${i + 1}.` : "•"}
            </Text>
            <View className="flex-1">
              <InlineText text={item} tone={tone} />
            </View>
          </View>
        ))}
      </View>
    );
  }
  return <InlineText text={block.text} tone={tone} />;
}

function Heading({
  level,
  text,
  tone
}: {
  level: 1 | 2 | 3;
  text: string;
  tone: "default" | "onAccent";
}) {
  const sizes: Record<1 | 2 | 3, number> = { 1: 20, 2: 17, 3: 15 };
  const baseColor =
    tone === "onAccent" ? "#06070A" : "#F8FAFC";
  return (
    <Text
      className={cn("font-semibold")}
      style={{
        fontSize: sizes[level],
        lineHeight: sizes[level] + 4,
        // RN-web sometimes drops Tailwind font-weight on a deeply
        // nested Text; the inline style is the belt that pairs with
        // the className suspenders.
        fontWeight: "700",
        color: baseColor
      }}
    >
      {text}
    </Text>
  );
}

// InlineText renders a paragraph with inline markdown applied. We
// nest emphasis as child <Text> elements with explicit `style` so
// fontWeight/fontFamily survive RN-web's text-style inheritance
// quirks.
function InlineText({
  text,
  tone,
  italic = false
}: {
  text: string;
  tone: "default" | "onAccent";
  italic?: boolean;
}) {
  const segments = parseInline(text);
  const baseClass = tone === "onAccent" ? "text-background" : "text-foreground/95";
  return (
    <Text
      variant="small"
      className={baseClass}
      style={italic ? { fontStyle: "italic", lineHeight: 20 } : { lineHeight: 20 }}
    >
      {segments.map((seg, i) => (
        <Fragment key={i}>{renderSegment(seg, tone)}</Fragment>
      ))}
    </Text>
  );
}

type Segment =
  | { kind: "text"; text: string }
  | { kind: "code"; text: string }
  | { kind: "bold"; text: string }
  | { kind: "italic"; text: string }
  | { kind: "link"; text: string; href: string };

// parseInline tokenizes a paragraph into Segments. We use a small
// hand-written scanner instead of a regex `replace` chain because
// the chains kept eating characters from adjacent matches (e.g. a
// `code` span followed immediately by **bold**).
function parseInline(input: string): Segment[] {
  const out: Segment[] = [];
  let i = 0;
  let buf = "";

  const flushText = () => {
    if (buf.length > 0) {
      out.push({ kind: "text", text: buf });
      buf = "";
    }
  };

  while (i < input.length) {
    const rest = input.slice(i);

    // Inline code `…`.
    if (rest[0] === "`") {
      const end = rest.indexOf("`", 1);
      if (end > 1) {
        flushText();
        out.push({ kind: "code", text: rest.slice(1, end) });
        i += end + 1;
        continue;
      }
    }

    // Bold **…** — checked BEFORE italic so `**` doesn't get split
    // into two `*` italics.
    if (rest.startsWith("**")) {
      const end = rest.indexOf("**", 2);
      if (end > 2) {
        flushText();
        out.push({ kind: "bold", text: rest.slice(2, end) });
        i += end + 2;
        continue;
      }
    }

    // Italic _…_ or *…*.
    //
    // Tricky case: tool / identifier names like `reddit_subreddit_about`
    // contain underscores that are NOT meant to be italic markers.
    // Standard CommonMark handles this by forbidding intra-word `_`
    // emphasis (the boundary character on the outside must be
    // non-alphanumeric). We follow that rule for `_` and only enforce
    // the whitespace check for `*` — operators routinely write
    // `*verb*` mid-sentence, so we want that to italicize.
    if (rest[0] === "_" || rest[0] === "*") {
      const marker = rest[0];
      const end = rest.indexOf(marker, 1);
      if (end > 1 && rest[1] !== " " && rest[end - 1] !== " ") {
        const prev = i > 0 ? input[i - 1] : "";
        const next = end + 1 < rest.length ? rest[end + 1] : "";
        const isAlnum = (c: string) => /[A-Za-z0-9]/.test(c);
        const intraWord =
          marker === "_" && (isAlnum(prev) || isAlnum(next));
        if (!intraWord) {
          flushText();
          out.push({ kind: "italic", text: rest.slice(1, end) });
          i += end + 1;
          continue;
        }
      }
    }

    // Markdown link [text](url).
    if (rest[0] === "[") {
      const linkMatch = /^\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/.exec(rest);
      if (linkMatch) {
        flushText();
        out.push({ kind: "link", text: linkMatch[1], href: linkMatch[2] });
        i += linkMatch[0].length;
        continue;
      }
    }

    // Bare URL — match http(s)://… up to whitespace or terminal punctuation.
    if (rest.startsWith("http://") || rest.startsWith("https://")) {
      const urlMatch = /^https?:\/\/[^\s)<>"]+/.exec(rest);
      if (urlMatch) {
        let url = urlMatch[0];
        const trailing = /[.,!?;:]$/.exec(url);
        if (trailing) url = url.slice(0, -1);
        flushText();
        out.push({ kind: "link", text: url, href: url });
        i += url.length;
        continue;
      }
    }

    buf += input[i];
    i++;
  }
  flushText();
  return out;
}

function renderSegment(seg: Segment, tone: "default" | "onAccent") {
  if (seg.kind === "text") return seg.text;
  if (seg.kind === "code") {
    // Inline code uses inline style for fontFamily because RN-web
    // doesn't always inherit font-family from a className on a
    // nested <Text>.
    return (
      <Text
        style={{
          fontFamily: monoFont(),
          fontSize: 12,
          color: tone === "onAccent" ? "#06070A" : "#F8FAFC"
        }}
      >
        {seg.text}
      </Text>
    );
  }
  if (seg.kind === "bold") {
    return (
      <Text
        style={{
          fontWeight: "700",
          color: tone === "onAccent" ? "#06070A" : "#F8FAFC"
        }}
      >
        {seg.text}
      </Text>
    );
  }
  if (seg.kind === "italic") {
    return (
      <Text
        style={{
          fontStyle: "italic",
          color: tone === "onAccent" ? "#06070A" : "#F8FAFC"
        }}
      >
        {seg.text}
      </Text>
    );
  }
  return <LinkText href={seg.href} label={seg.text} tone={tone} />;
}

function LinkText({
  href,
  label,
  tone
}: {
  href: string;
  label: string;
  tone: "default" | "onAccent";
}) {
  const onPress = () => {
    Linking.openURL(href).catch(() => {
      /* swallow — opening failed (no handler / blocked); not worth alerting */
    });
  };
  return (
    <Text
      onPress={onPress}
      style={{
        textDecorationLine: "underline",
        color: tone === "onAccent" ? "#06070A" : "#00D4AA"
      }}
    >
      {label}
    </Text>
  );
}

// MarkdownImage renders a standalone `![alt](url)` block. The chat
// composer emits one of these per attached photo (after the upload
// resolves to a signed URL) so the operator's bubble shows a real
// preview instead of a raw URL. We cap the height so a portrait
// phone shot doesn't push the message text out of view; tapping
// opens the signed URL in the OS image viewer / browser tab.
function MarkdownImage({
  url,
  alt,
  tone
}: {
  url: string;
  alt: string;
  tone: "default" | "onAccent";
}) {
  const onPress = () => {
    Linking.openURL(url).catch(() => {
      /* swallow */
    });
  };
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="image"
      accessibilityLabel={alt || "attached image"}
    >
      <Image
        source={{ uri: url }}
        style={{
          width: "100%",
          height: 240,
          borderRadius: 12,
          backgroundColor: "#000"
        }}
        resizeMode="cover"
      />
      {alt ? (
        <Text
          variant="muted"
          className={cn(
            "mt-1 text-xs",
            tone === "onAccent" ? "text-background/80" : "text-foreground/60"
          )}
        >
          {alt}
        </Text>
      ) : null}
    </Pressable>
  );
}

// ExternalLinkButton is the button-shaped variant the per-tool
// result cards use ("View on Reddit", "Open comment"). Kept here
// because Markdown is the import everything-chat-related package.
export function ExternalLinkButton({ href, label }: { href: string; label: string }) {
  return (
    <Pressable
      onPress={() =>
        Linking.openURL(href).catch(() => {
          /* swallow */
        })
      }
      className="self-start rounded-lg border border-primary/40 bg-primary/10 px-3 py-1.5 active:bg-primary/20"
    >
      <Text variant="small" className="text-primary">
        {label}
      </Text>
    </Pressable>
  );
}

function monoFont(): string {
  // RN doesn't ship a guaranteed monospace family, but these resolve
  // to a sensible default per-platform. iOS picks Menlo, Android
  // picks monospace, web falls back to the browser monospace stack.
  return "Menlo, Consolas, monospace";
}
