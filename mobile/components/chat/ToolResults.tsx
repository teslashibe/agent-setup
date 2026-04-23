import { useMemo, useState } from "react";
import { Pressable, View } from "react-native";
import { ChevronDown, ChevronRight } from "lucide-react-native";

import { Text } from "@/components/ui/Text";
import { ExternalLinkButton, Markdown } from "@/components/chat/Markdown";

// Tool-specific renderers for MCP tool results. Each renderer takes the
// already-parsed result object and produces a presentation that's
// actually readable (vs. one-line truncated JSON).
//
// The `output` shape from the SSE stream is whatever the MCP tool
// returned. Anthropic wraps text-y results as
// `[{ type: "text", text: "<json string>" }]`, so we have a small
// `decodeMcpOutput` that unwraps that shape and JSON-parses the inner
// payload. Renderers receive the decoded value and key off `tool.name`.
//
// The dispatcher (`ToolResult`) has hand-tuned cards for the platform
// tools the template's MCP server ships today (reddit_*, nextdoor_*).
// Tools we don't have a custom renderer for fall through to
// `RawJsonCard`, which preserves indentation and is collapsed by
// default. That fallback is good enough to ship a new MCP tool
// without UI work — add a tuned card here when a tool gets used
// often enough that the JSON dump becomes the chat's bottleneck.

export type ToolResultProps = {
  toolName: string;
  output: unknown;
};

export function ToolResult({ toolName, output }: ToolResultProps) {
  const decoded = useMemo(() => decodeMcpOutput(output), [output]);

  switch (toolName) {
    case "reddit_subreddit_about":
      return <SubredditAboutCard data={decoded} />;
    case "reddit_subreddit_rules":
      return <SubredditRulesCard data={decoded} />;
    case "reddit_post_info":
      return <PostInfoCard post={decoded} />;
    case "reddit_posts_info":
      return <PostsInfoCard posts={decoded} />;
    case "reddit_post_insights":
      return <PostInsightsCard insights={decoded} />;
    case "reddit_submit":
    case "reddit_submit_link":
      return <SubmitResultCard data={decoded} />;
    case "nextdoor_get_me":
      return <NextdoorMeCard me={decoded} />;
    case "nextdoor_create_post":
      return <NextdoorPostCard data={decoded} />;
    default:
      return <RawJsonCard value={decoded ?? output} />;
  }
}

function SubredditAboutCard({ data }: { data: any }) {
  if (!isObject(data)) return <RawJsonCard value={data} />;
  const name = stringy(data.display_name ?? data.Name ?? data.name);
  const title = stringy(data.title ?? data.Title);
  const desc = stringy(data.public_description ?? data.Description);
  const subs = numby(data.subscribers ?? data.Subscribers);
  const rules = arrayish(data.rules ?? data.Rules);
  const siteRules = arrayish(data.site_rules ?? data.SiteRules);

  return (
    <View className="gap-2">
      <View className="flex-row items-baseline justify-between gap-2">
        <Text variant="small" className="font-semibold text-foreground">
          r/{name}
          {title ? ` — ${title}` : ""}
        </Text>
        {subs !== null ? (
          <Text variant="muted" className="text-xs">
            {formatNumber(subs)} subscribers
          </Text>
        ) : null}
      </View>
      {desc ? (
        <Text variant="muted" className="text-xs italic">
          {desc}
        </Text>
      ) : null}
      {rules.length > 0 ? (
        <View className="gap-1">
          <Text variant="small" className="font-semibold text-foreground">
            Rules ({rules.length})
          </Text>
          {rules.map((r, i) => (
            <RuleRow key={i} rule={r} index={i + 1} />
          ))}
        </View>
      ) : null}
      {siteRules.length > 0 ? (
        <View className="gap-0.5">
          <Text variant="muted" className="text-xs font-semibold">
            Site-wide rules
          </Text>
          {siteRules.map((r, i) => (
            <Text key={i} variant="muted" className="text-xs">
              • {String(r)}
            </Text>
          ))}
        </View>
      ) : null}
    </View>
  );
}

function SubredditRulesCard({ data }: { data: any }) {
  if (!isObject(data)) return <RawJsonCard value={data} />;
  const rules = arrayish(data.rules ?? data.Rules);
  const siteRules = arrayish(data.site_rules ?? data.SiteRules);
  return (
    <View className="gap-2">
      {rules.length > 0 ? (
        <View className="gap-1">
          {rules.map((r, i) => (
            <RuleRow key={i} rule={r} index={i + 1} />
          ))}
        </View>
      ) : (
        <Text variant="muted" className="text-xs">
          No moderator rules.
        </Text>
      )}
      {siteRules.length > 0 ? (
        <View className="gap-0.5 border-t border-border pt-2">
          <Text variant="muted" className="text-xs font-semibold">
            Site-wide rules
          </Text>
          {siteRules.map((r, i) => (
            <Text key={i} variant="muted" className="text-xs">
              • {String(r)}
            </Text>
          ))}
        </View>
      ) : null}
    </View>
  );
}

function RuleRow({ rule, index }: { rule: any; index: number }) {
  if (!isObject(rule)) return null;
  const shortName = stringy(rule.short_name ?? rule.ShortName);
  const description = stringy(rule.description ?? rule.Description);
  const kind = stringy(rule.kind ?? rule.Kind);
  return (
    <View className="gap-0.5">
      <View className="flex-row items-baseline gap-1.5">
        <Text variant="small" className="text-foreground/70">
          {index}.
        </Text>
        <Text variant="small" className="font-semibold text-foreground">
          {shortName || "(unnamed)"}
        </Text>
        {kind && kind !== "all" ? (
          <Text variant="muted" className="text-[10px] uppercase tracking-wider">
            ({kind})
          </Text>
        ) : null}
      </View>
      {description ? (
        <Text variant="muted" className="text-xs ml-4">
          {description}
        </Text>
      ) : null}
    </View>
  );
}

function PostInfoCard({ post }: { post: any }) {
  if (!isObject(post)) return <RawJsonCard value={post} />;
  return (
    <View className="gap-2">
      <PostMetricsRow post={post} />
      <PostHeader post={post} />
    </View>
  );
}

function PostsInfoCard({ posts }: { posts: any }) {
  if (!Array.isArray(posts) || posts.length === 0) {
    return (
      <Text variant="muted" className="text-xs">
        No posts returned.
      </Text>
    );
  }
  return (
    <View className="gap-3">
      {posts.map((p, i) => (
        <View key={i} className={i > 0 ? "border-t border-border pt-2" : undefined}>
          <PostInfoCard post={p} />
        </View>
      ))}
    </View>
  );
}

function PostMetricsRow({ post }: { post: any }) {
  const score = numby(post.score ?? post.Score);
  const ratio = numby(post.upvote_ratio ?? post.UpvoteRatio);
  const comments = numby(post.num_comments ?? post.NumComments);
  const views = numby(post.view_count ?? post.ViewCount);
  return (
    <View className="flex-row flex-wrap gap-x-4 gap-y-1">
      {score !== null ? <Metric label="score" value={formatNumber(score)} /> : null}
      {ratio !== null ? <Metric label="upvote ratio" value={`${Math.round(ratio * 100)}%`} /> : null}
      {comments !== null ? <Metric label="comments" value={formatNumber(comments)} /> : null}
      {views !== null ? <Metric label="views" value={formatNumber(views)} /> : null}
    </View>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <View>
      <Text variant="muted" className="text-[10px] uppercase tracking-wider">
        {label}
      </Text>
      <Text variant="small" className="font-semibold text-foreground">
        {value}
      </Text>
    </View>
  );
}

function PostHeader({ post }: { post: any }) {
  const title = stringy(post.title ?? post.Title);
  const sub = stringy(post.subreddit ?? post.Subreddit);
  const permalink = stringy(post.permalink ?? post.Permalink);
  const url = permalink ? `https://reddit.com${permalink}` : stringy(post.url ?? post.URL);
  return (
    <View className="gap-1">
      {title ? (
        <Text variant="small" className="font-semibold text-foreground">
          {title}
        </Text>
      ) : null}
      {sub ? (
        <Text variant="muted" className="text-xs">
          r/{sub}
        </Text>
      ) : null}
      {url ? <ExternalLinkButton href={url} label="View on Reddit" /> : null}
    </View>
  );
}

// PostInsightsCard renders the author-only analytics view from
// reddit_post_insights. The hero metric is total views (Reddit's
// formatted "51K" beats the raw integer because the integer is
// derived from the 48-hour chart and may undercount older posts).
// Ribbon comes from PersonalComparison ("#1 of all time") and is
// optional. Hourly chart is rendered as a unicode sparkline so it
// fits inline in chat without needing a charting lib.
function PostInsightsCard({ insights }: { insights: any }) {
  if (!isObject(insights)) return <RawJsonCard value={insights} />;

  const title = stringy(insights.title ?? insights.Title);
  const sub = stringy(insights.subreddit ?? insights.Subreddit);
  const permalink = stringy(insights.permalink ?? insights.Permalink);
  const ribbon = stringy(
    insights.personal_comparison ?? insights.PersonalComparison,
  );

  const totalViewsFormatted = stringy(
    insights.total_views_formatted ?? insights.TotalViewsFormatted,
  );
  const totalViews = numby(insights.total_views ?? insights.TotalViews);
  const viewsChange = stringy(
    insights.views_change_formatted ?? insights.ViewsChangeFormatted,
  );

  const upvotes = numby(insights.upvotes ?? insights.Upvotes);
  const upvoteRatio = numby(insights.upvote_ratio ?? insights.UpvoteRatio);
  const comments = numby(insights.comments ?? insights.Comments);
  const shares = numby(insights.shares ?? insights.Shares);
  const crossposts = numby(insights.crossposts ?? insights.Crossposts);
  const awards = numby(insights.awards ?? insights.Awards);

  const hourly = arrayish(insights.hourly_views ?? insights.HourlyViews);
  const topComments = arrayish(insights.top_comments ?? insights.TopComments);

  const headlineViews =
    totalViewsFormatted ||
    (totalViews !== null ? formatNumber(totalViews) : "—");

  return (
    <View className="gap-3">
      {title ? (
        <View className="gap-0.5">
          <Text variant="small" className="font-semibold text-foreground">
            {title}
          </Text>
          {sub ? (
            <Text variant="muted" className="text-xs">
              r/{sub}
            </Text>
          ) : null}
        </View>
      ) : null}

      {ribbon ? (
        <View className="rounded-md bg-foreground/5 px-2 py-1.5">
          <Text variant="small" className="text-foreground">
            {ribbon}
          </Text>
        </View>
      ) : null}

      <View className="flex-row items-baseline gap-2">
        <Text className="text-3xl font-bold text-foreground">
          {headlineViews}
        </Text>
        <Text variant="muted" className="text-xs uppercase tracking-wider">
          views
        </Text>
        {viewsChange ? (
          <Text variant="muted" className="text-xs">
            {viewsChange} (24h)
          </Text>
        ) : null}
      </View>

      <View className="flex-row flex-wrap gap-x-4 gap-y-1">
        {upvotes !== null ? (
          <Metric label="upvotes" value={formatNumber(upvotes)} />
        ) : null}
        {upvoteRatio !== null ? (
          <Metric label="ratio" value={`${Math.round(upvoteRatio * 100)}%`} />
        ) : null}
        {comments !== null ? (
          <Metric label="comments" value={formatNumber(comments)} />
        ) : null}
        {shares !== null ? (
          <Metric label="shares" value={formatNumber(shares)} />
        ) : null}
        {crossposts !== null && crossposts > 0 ? (
          <Metric label="crossposts" value={formatNumber(crossposts)} />
        ) : null}
        {awards !== null && awards > 0 ? (
          <Metric label="awards" value={formatNumber(awards)} />
        ) : null}
      </View>

      {hourly.length > 0 ? <HourlySparkline points={hourly} /> : null}

      {topComments.length > 0 ? (
        <View className="gap-2 border-t border-border pt-2">
          <Text variant="muted" className="text-[10px] uppercase tracking-wider">
            Top comments
          </Text>
          {topComments.map((c, i) => (
            <TopCommentRow key={i} comment={c} />
          ))}
        </View>
      ) : null}

      {permalink ? (
        <ExternalLinkButton href={permalink} label="View on Reddit" />
      ) : null}
    </View>
  );
}

// HourlySparkline draws a 1-line bar chart of the per-hour view
// breakdown using unicode block characters. Each bar is normalized
// against the chart's max so the relative shape matches Reddit's
// own chart, even though we lose the exact y-axis. Below the bars
// we show a tiny axis label ("h1 → h48") plus the peak hour so the
// agent can reference it ("most views came at hour 2").
function HourlySparkline({ points }: { points: any[] }) {
  const data = points
    .map((p) => ({
      hour: numby(p.hour_offset ?? p.HourOffset) ?? 0,
      views: numby(p.views ?? p.Views) ?? 0,
    }))
    .filter((p) => p.hour > 0);
  if (data.length === 0) return null;
  const max = Math.max(...data.map((p) => p.views));
  if (max === 0) return null;
  const blocks = ["▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"];
  const bars = data
    .map((p) => {
      const n = Math.min(blocks.length - 1, Math.floor((p.views / max) * blocks.length));
      return blocks[n];
    })
    .join("");
  const peak = data.reduce((a, b) => (b.views > a.views ? b : a), data[0]);
  return (
    <View className="gap-1">
      <Text variant="muted" className="text-[10px] uppercase tracking-wider">
        Hourly views (first {data.length}h)
      </Text>
      <Text className="text-foreground" style={{ fontFamily: "Menlo, Consolas, monospace", letterSpacing: 1 }}>
        {bars}
      </Text>
      <Text variant="muted" className="text-xs">
        peak: hour {peak.hour} ({formatNumber(peak.views)} views)
      </Text>
    </View>
  );
}

function TopCommentRow({ comment }: { comment: any }) {
  if (!isObject(comment)) return null;
  const author = stringy(comment.author ?? comment.Author);
  const score = numby(comment.score ?? comment.Score);
  const body = stringy(comment.body ?? comment.Body);
  const permalink = stringy(comment.permalink ?? comment.Permalink);
  return (
    <View className="gap-0.5">
      <View className="flex-row items-baseline gap-2">
        <Text variant="small" className="font-semibold text-foreground">
          u/{author || "(deleted)"}
        </Text>
        {score !== null ? (
          <Text variant="muted" className="text-xs">
            {formatNumber(score)} pts
          </Text>
        ) : null}
      </View>
      {body ? (
        <Text variant="small" className="text-foreground/80" numberOfLines={4}>
          {body}
        </Text>
      ) : null}
      {permalink ? <ExternalLinkButton href={permalink} label="Open comment" /> : null}
    </View>
  );
}

function SubmitResultCard({ data }: { data: any }) {
  if (!isObject(data)) return <RawJsonCard value={data} />;
  const url = stringy(data.url ?? data.URL);
  const id = stringy(data.id ?? data.ID);
  const sub = stringy(data.subreddit ?? data.Subreddit);
  return (
    <View className="gap-2">
      <Text variant="small" className="font-semibold text-foreground">
        Posted{sub ? ` to r/${sub}` : ""}
      </Text>
      {id ? (
        <Text variant="muted" className="text-xs">
          id: {id}
        </Text>
      ) : null}
      {url ? <ExternalLinkButton href={url} label="View on Reddit" /> : null}
    </View>
  );
}

function NextdoorMeCard({ me }: { me: any }) {
  if (!isObject(me)) return <RawJsonCard value={me} />;
  const display =
    stringy(me.display_name ?? me.DisplayName) ||
    stringy(me.given_name ?? me.GivenName) ||
    "(no display name)";
  const hood = stringy(me.neighborhood_name ?? me.NeighborhoodName) || "(no neighborhood)";
  return (
    <View className="gap-1">
      <Text variant="small" className="font-semibold text-foreground">
        {display}
      </Text>
      <Text variant="muted" className="text-xs">
        Posting into: {hood}
      </Text>
    </View>
  );
}

function NextdoorPostCard({ data }: { data: any }) {
  if (!isObject(data)) return <RawJsonCard value={data} />;
  const id = stringy(data.id ?? data.post_id);
  return (
    <View className="gap-1">
      <Text variant="small" className="font-semibold text-foreground">
        Posted to Nextdoor
      </Text>
      {id ? (
        <Text variant="muted" className="text-xs">
          id: {id}
        </Text>
      ) : null}
    </View>
  );
}

// RawJsonCard is the fallback for tools we don't have a custom renderer
// for. It's collapsed by default to a one-line preview; tap to expand.
// Unlike the previous "output: {…}" line, this preserves indentation
// and survives long payloads.
export function RawJsonCard({ value }: { value: unknown }) {
  const [open, setOpen] = useState(false);
  const pretty = useMemo(() => prettyJson(value), [value]);
  const preview = useMemo(() => oneLinePreview(value), [value]);

  return (
    <View className="gap-1">
      <Pressable
        onPress={() => setOpen((o) => !o)}
        className="flex-row items-center gap-1"
        hitSlop={6}
      >
        {open ? <ChevronDown size={12} color="#9AA4B2" /> : <ChevronRight size={12} color="#9AA4B2" />}
        <Text variant="muted" className="text-xs flex-1" numberOfLines={1}>
          {open ? "result" : preview}
        </Text>
      </Pressable>
      {open ? (
        <View className="rounded-lg border border-border bg-background/40 px-3 py-2">
          <Text className="text-xs text-foreground/90" style={{ fontFamily: "Menlo, Consolas, monospace" }}>
            {pretty}
          </Text>
        </View>
      ) : null}
    </View>
  );
}

// ToolInputPreview is the typed-input renderer used at the top of every
// tool card — same JSON-preview-with-expand UX as the result, scoped
// to the tool input so the operator can audit what the agent passed.
export function ToolInputPreview({ input }: { input: unknown }) {
  if (input === undefined || input === null) return null;
  // For simple string-only objects (the common case for our tools),
  // render inline like `subreddit: sanfrancisco` rather than a chevron.
  if (isObject(input)) {
    const entries = Object.entries(input).filter(([, v]) => v !== undefined);
    if (entries.length === 0) return null;
    if (entries.every(([, v]) => typeof v === "string" || typeof v === "number" || typeof v === "boolean")) {
      return (
        <Text variant="muted" className="text-xs">
          {entries.map(([k, v]) => `${k}: ${String(v)}`).join("  •  ")}
        </Text>
      );
    }
  }
  return <RawJsonCard value={input} />;
}

// decodeMcpOutput pierces the wrapping Anthropic puts around MCP tool
// results. Real Anthropic responses use:
//   [{ type: "text", text: "<JSON string>" }]
// for tool results. Some of our tools also emit a plain object directly
// (e.g. when invoked from session history that we re-translated). We
// try both: if the input is the wrapped shape, parse the inner text.
export function decodeMcpOutput(value: unknown): unknown {
  if (value == null) return value;
  if (Array.isArray(value)) {
    // Concatenate all text blocks; if there's only one and it parses as
    // JSON, return the parsed value.
    const texts = value
      .filter((b: any) => b && typeof b === "object" && b.type === "text" && typeof b.text === "string")
      .map((b: any) => b.text as string);
    if (texts.length === 0) return value;
    const joined = texts.join("");
    try {
      return JSON.parse(joined);
    } catch {
      return joined;
    }
  }
  if (isObject(value) && typeof (value as any).text === "string") {
    try {
      return JSON.parse((value as any).text);
    } catch {
      return (value as any).text;
    }
  }
  return value;
}

function prettyJson(v: unknown): string {
  if (v === undefined) return "(no result)";
  if (v === null) return "null";
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return String(v);
  }
}

function oneLinePreview(v: unknown): string {
  if (v === undefined) return "(no result)";
  if (v === null) return "null";
  if (typeof v === "string") {
    return v.length > 80 ? v.slice(0, 80) + "…" : v;
  }
  try {
    const s = JSON.stringify(v);
    return s.length > 80 ? s.slice(0, 80) + "…" : s;
  } catch {
    return String(v);
  }
}

function isObject(v: unknown): v is Record<string, any> {
  return v !== null && typeof v === "object" && !Array.isArray(v);
}

function stringy(v: unknown): string {
  return typeof v === "string" ? v : v == null ? "" : String(v);
}

function numby(v: unknown): number | null {
  if (typeof v === "number" && !Number.isNaN(v)) return v;
  if (typeof v === "string" && v.trim() !== "") {
    const n = Number(v);
    if (!Number.isNaN(n)) return n;
  }
  return null;
}

function arrayish(v: unknown): any[] {
  return Array.isArray(v) ? v : [];
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 10_000) return `${Math.round(n / 1_000)}k`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

// Markdown re-export so chat/[id].tsx can import everything chat-related
// from one place. Keeps the import block in the screen short.
export { Markdown };
