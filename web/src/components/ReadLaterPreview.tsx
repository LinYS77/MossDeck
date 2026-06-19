import type { ReadLaterItem } from "../lib/types";
import { READ_LATER } from "../features/home/mockData";
import { GlassPanel } from "./GlassPanel";
import { SectionHeader } from "./SectionHeader";
import { BookmarkIcon, ClockIcon, HeartIcon } from "./icons";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./ReadLaterPreview.module.css";

/** "Read later" queue preview shown in the sidebar and on home. */
export function ReadLaterPreview({
  items = READ_LATER,
  onSeeAll,
}: {
  items?: ReadLaterItem[];
  onSeeAll?: () => void;
}) {
  const { t } = useI18n();

  return (
    <GlassPanel as="section" className={styles.panel}>
      <SectionHeader
        icon={<BookmarkIcon />}
        title={t("readlaterPreview.title")}
        actionLabel={t("readlaterPreview.seeAll")}
        onAction={onSeeAll}
      />

      <ul className={styles.list}>
        {items.map((item) => (
          <li key={item.id}>
            <a
              className={styles.item}
              href={item.url}
              target="_blank"
              rel="noreferrer noopener"
            >
              <div className={styles.body}>
                <div className={styles.titleRow}>
                  {item.favorite ? (
                    <HeartIcon className={styles.heart} />
                  ) : null}
                  <span className={styles.title}>{item.title}</span>
                </div>
                <div className={styles.meta}>
                  <span className={styles.source}>{item.source ?? item.domain}</span>
                  <span className={styles.dot} aria-hidden>·</span>
                  <span className={styles.reading}>
                    <ClockIcon className={styles.clock} />
                    {item.readingTime} {t("card.min")}
                  </span>
                </div>
              </div>
            </a>
          </li>
        ))}
      </ul>
    </GlassPanel>
  );
}
