import type {
  BookmarkGroup,
  QuickLink,
  ReadLaterItem,
  SearchEngineId,
} from "../../lib/types";

/** Search engine options for the hero search bar. The query is appended as a
 * URL search param when the user submits. */
export const SEARCH_ENGINES: {
  id: SearchEngineId;
  label: string;
  url: (q: string) => string;
}[] = [
  { id: "duckduckgo", label: "DuckDuckGo", url: (q) => `https://duckduckgo.com/?q=${encodeURIComponent(q)}` },
  { id: "google", label: "Google", url: (q) => `https://www.google.com/search?q=${encodeURIComponent(q)}` },
];

/** Frequently used shortcuts shown as icon tiles under the search bar. */
export const QUICK_LINKS: QuickLink[] = [
  { id: "1", label: "GitHub", url: "https://github.com", icon: "GH", color: "#8b5cf6" },
  { id: "2", label: "YouTube", url: "https://youtube.com", icon: "YT", color: "#ef4444" },
  { id: "3", label: "Gmail", url: "https://mail.google.com", icon: "GM", color: "#22d3ee" },
  { id: "4", label: "X", url: "https://x.com", icon: "X", color: "#6366f1" },
  { id: "5", label: "Reddit", url: "https://reddit.com", icon: "RD", color: "#f97316" },
  { id: "6", label: "Hacker News", url: "https://news.ycombinator.com", icon: "HN", color: "#eab308" },
  { id: "7", label: "Figma", url: "https://figma.com", icon: "FG", color: "#ec4899" },
  { id: "8", label: "Notion", url: "https://notion.so", icon: "NT", color: "#14b8a6" },
];

/** Bookmark groups preview (the "分组" the user files things under). */
export const BOOKMARK_GROUPS: BookmarkGroup[] = [
  {
    id: "dev",
    name: "Development",
    icon: "code",
    bookmarks: [
      { id: "b1", title: "Go Documentation", url: "https://go.dev/doc", domain: "go.dev", description: "The Go programming language docs.", color: "#00add8" },
      { id: "b2", title: "MDN Web Docs", url: "https://developer.mozilla.org", domain: "developer.mozilla.org", description: "Resources for developers, by developers.", color: "#000000" },
      { id: "b3", title: "TypeScript Handbook", url: "https://www.typescriptlang.org/docs", domain: "typescriptlang.org", color: "#3178c6" },
      { id: "b4", title: "React Docs", url: "https://react.dev", domain: "react.dev", color: "#61dafb" },
    ],
  },
  {
    id: "design",
    name: "Design & Inspiration",
    icon: "sparkles",
    bookmarks: [
      { id: "b5", title: "Dribbble", url: "https://dribbble.com", domain: "dribbble.com", description: "Discover the world's top designers.", color: "#ea4c89" },
      { id: "b6", title: "Awwwards", url: "https://www.awwwards.com", domain: "awwwards.com", color: "#000000" },
      { id: "b7", title: "Coolors", url: "https://coolors.co", domain: "coolors.co", description: "The super fast color palette generator.", color: "#ff6b6b" },
    ],
  },
  {
    id: "read",
    name: "Reading",
    icon: "book",
    bookmarks: [
      { id: "b8", title: "Hacker News", url: "https://news.ycombinator.com", domain: "news.ycombinator.com", color: "#ff6600" },
      { id: "b9", title: "The Verge", url: "https://www.theverge.com", domain: "theverge.com", color: "#5200ff" },
      { id: "b10", title: "Ars Technica", url: "https://arstechnica.com", domain: "arstechnica.com", color: "#ff4e00" },
    ],
  },
];

/** Read-later queue preview. */
export const READ_LATER: ReadLaterItem[] = [
  { id: "r1", title: "Designing for the new-tab moment", url: "https://example.com/new-tab", domain: "example.com", readingTime: 6, source: "Smashing", favorite: true },
  { id: "r2", title: "A practical guide to glassmorphism", url: "https://example.com/glass", domain: "example.com", readingTime: 9, source: "CSS-Tricks" },
  { id: "r3", title: "Why SQLite is more than enough", url: "https://example.com/sqlite", domain: "example.com", readingTime: 12, source: "Fly.io" },
  { id: "r4", title: "The art of the loading state", url: "https://example.com/loading", domain: "example.com", readingTime: 4, source: "A List Apart" },
];

export const USER_NAME = "Winnie";
