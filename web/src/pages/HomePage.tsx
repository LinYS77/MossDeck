import { Clock } from "../components/Clock";
import { SearchBar } from "../components/SearchBar";
import { QuickLinks } from "../components/QuickLinks";
import { SectionHeader } from "../components/SectionHeader";
import { BookmarkGroupsGrid } from "../components/BookmarkGroup";
import { ReadLaterPreview } from "../components/ReadLaterPreview";
import { WeatherWidget, NotesWidget } from "../components/Widgets";
import { EmptyState } from "../components/EmptyState";
import { GlassSkeleton } from "../components/GlassSkeleton";
import {
  LayersIcon,
  BookmarkIcon,
} from "../components/icons";
import { Link, useNavigate } from "react-router-dom";
import { useHomeData } from "../features/home/useHomeData";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./HomePage.module.css";

/** Home / new-tab view. Reads live data from the API and maps it into the
 *  existing glass UI. Loading uses skeletons; empty data shows tasteful empty
 *  states; per-section errors degrade gracefully. */
export function HomePage() {
  const navigate = useNavigate();
  const data = useHomeData();
  const { t } = useI18n();

  const isLoading = data.status === "loading";
  const hasBookmarks = data.groups.length > 0;

  return (
    <div className={styles.page}>
      <main className={styles.main}>
        {/* Hero: clock + search + quick links */}
        <section className={styles.hero}>
          <Clock />
          <SearchBar />
          {data.quickLinks.length > 0 ? (
            <div className={styles.quickLinks}>
              <QuickLinks
                items={data.quickLinks}
                onRecordOpen={data.recordQuickLinkOpen}
              />
            </div>
          ) : null}
        </section>

        {/* Read later — a dedicated horizontal strip between hero and content */}
        <section className={styles.readLaterStrip}>
          <ReadLaterPreview
            items={data.readLater}
            onSeeAll={() => navigate("/read-later")}
            onAdded={() => data.reload(true)}
          />
        </section>

        {/* Two-column: bookmarks (left) + widgets (right) */}
        <div className={styles.layout}>
          <section className={styles.primary}>
            <SectionHeader
              icon={<LayersIcon />}
              title={t("home.bookmarks")}
              actionLabel={t("home.manage")}
              onAction={() => navigate("/bookmarks")}
            />

            {isLoading ? (
              <div className={styles.skeletonGrid}>
                <GlassSkeleton lines={4} />
                <GlassSkeleton lines={3} />
              </div>
            ) : hasBookmarks ? (
              <BookmarkGroupsGrid groups={data.groups} />
            ) : (
              <EmptyState
                icon={<BookmarkIcon />}
                title={t("home.noBookmarks")}
                description={t("home.noBookmarksDesc")}
                action={
                  <Link to="/bookmarks" className={styles.manageLink}>
                    {t("home.manageBookmarks")}
                  </Link>
                }
              />
            )}
          </section>

          <aside className={styles.aside}>
            {isLoading ? (
              <>
                <GlassSkeleton lines={4} />
                <GlassSkeleton lines={2} />
              </>
            ) : (
              <>
                <WeatherWidget />
                <NotesWidget />
              </>
            )}
          </aside>
        </div>
      </main>

      <footer className={styles.footer}>
        <span>{t("topbar.homepage")}</span>
        <span className={styles.footerDot} aria-hidden>
          ·
        </span>
        <span>{t("common.connectedAPI")}</span>
      </footer>
    </div>
  );
}
