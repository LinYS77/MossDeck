-- 0004_category_show_on_home.sql
-- Adds a per-category "show on home" flag so the user can choose which
-- categories surface on the homepage's bookmark groups.
--
-- Defaults to 1 (show) for backward compatibility: every existing category
-- keeps appearing on the home page until the user explicitly hides it from
-- the category manager. The home-page grouping (mapBookmarksToGroups) filters
-- on this flag; the bookmarks management page ignores it (all categories are
-- always listed there).

ALTER TABLE categories ADD COLUMN show_on_home INTEGER NOT NULL DEFAULT 1;
