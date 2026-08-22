import type { Decision, DecisionKind } from "@/api"

// Severity carried on four axes at once, because colour alone cannot carry it.
//
// alert (#f6465d) and watch (#f0b90b) are adjacent tiers and are the worst pair
// in the palette for red-green colour vision deficiency — the one weakness the
// design brief named outright (§6.2). So the tier is also a glyph, also a word,
// also a bar of a different width, and also a position: tiers are grouped under
// their own heading rather than interleaved. Any one of those read on its own
// answers "how bad is this".
export type Tier = "alert" | "watch" | "info" | "neutral"

export type TierSpec = {
  /** Group heading. It says what to do, not what is wrong — the card below
   *  already says what is wrong. */
  label: string
  /** Second axis: shape. Distinct silhouettes, not shades of one mark. */
  glyph: string
  /** Third axis: weight of the card's left bar, 4px down to 1px. */
  bar: string
  text: string
  border: string
  /** Why this tier is where it is in the order. */
  note: string
}

export const TIER: Record<Tier, TierSpec> = {
  alert: {
    label: "Fix now",
    glyph: "▲",
    bar: "border-l-4 border-l-status-alert",
    text: "text-status-alert",
    border: "border-status-alert",
    note: "costs the reader right now",
  },
  watch: {
    label: "Verify",
    glyph: "◐",
    bar: "border-l-[3px] border-l-status-watch",
    text: "text-status-watch",
    border: "border-status-watch",
    note: "may be wrong — confirm",
  },
  info: {
    label: "Waiting on you",
    glyph: "◆",
    bar: "border-l-2 border-l-info",
    text: "text-info",
    border: "border-info",
    note: "blocked until you decide",
  },
  neutral: {
    label: "Signals",
    glyph: "·",
    bar: "border-l border-l-muted",
    text: "text-muted",
    border: "border-muted",
    note: "no cost until it is hit again",
  },
}

// The map from Go's eight kinds onto the four tiers. Yellow is absent from it
// on purpose: brand yellow belongs to actions and stat numbers, so severity
// runs on its own ramp (design-system.md §0.5).
export const TIER_OF: Record<DecisionKind, Tier> = {
  dangling: "alert",
  glossary_full: "alert",
  expired: "watch",
  drift: "watch",
  stale_skill: "watch",
  approve: "info",
  unread_set: "neutral",
  uncovered: "neutral",
}

export const TIER_ORDER: Tier[] = ["alert", "watch", "info", "neutral"]

/** The kind, spelled the way the card's gutter shows it. Go's identifiers with
 *  the underscore opened up — not translated, because these eight words are the
 *  same eight words the CLI and the docs use. */
export const kindLabel = (k: DecisionKind) => k.replace(/_/g, " ")

/** Backlog groups. The two neutral kinds are different enough that one heading
 *  over both would describe neither. */
export const BACKLOG_GROUPS: { kind: DecisionKind; label: string; note: string }[] = [
  { kind: "unread_set", label: "Unread sets", note: "presented and never opened" },
  { kind: "uncovered", label: "Uncovered questions", note: "asked and not answered" },
]

export const tierOf = (d: Decision) => TIER_OF[d.kind]

/** A decision is in the queue when it costs something today. Everything neutral
 *  is real and stays reachable — it just does not belong in front of the person
 *  who came here to ask what to do now. */
export const isQueue = (d: Decision) => TIER_OF[d.kind] !== "neutral"

/** Titles are node IDs, set names, canonicals — identifiers, read character by
 *  character — except where the title is a sentence somebody wrote. */
export const titleIsMono = (d: Decision) => d.kind !== "uncovered" && d.kind !== "glossary_full"

/** What the card's copy button puts on the clipboard: the most specific
 *  locator the decision has. */
export function locator(d: Decision, sets: { name: string; rel_path: string }[]): string {
  if (d.node_id) return d.node_id
  if (d.set) return sets.find((s) => s.name === d.set)?.rel_path ?? d.set
  return d.canonical ?? d.title
}

/** The chips under a card's detail. Every one of them is a field Go filled in;
 *  none is computed here beyond choosing the word in front of it. */
export function metaOf(d: Decision): string[] {
  const out: string[] = []
  if (d.set) out.push(`set ${d.set}`)
  if (d.role) out.push(`role ${d.role}`)
  switch (d.kind) {
    case "unread_set":
      // Count is how often the catalogue offered it. Opened is zero by the
      // definition of the kind, and saying so is the whole point of the pair.
      if (d.count) out.push(`${d.count} presented`, "0 opened")
      break
    case "approve":
      out.push("unverified")
      if (d.evidence?.length) out.push(`${d.evidence.length} citations`)
      if (d.count && d.count > 1) out.push(`proposed ×${d.count}`)
      break
    case "drift":
      if (d.count) out.push(`served stale ×${d.count}`)
      break
    default:
      if (d.count && d.count > 1) out.push(`×${d.count}`)
  }
  return out
}
