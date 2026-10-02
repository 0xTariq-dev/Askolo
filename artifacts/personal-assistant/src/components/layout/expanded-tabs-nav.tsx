import { useEffect, useRef, useState } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'framer-motion';
import type { ComponentType } from 'react';
import { cn } from '@/lib/utils';

export interface ExpandedNavTab {
  title: string;
  href: string;
  icon: ComponentType<{ className?: string }>;
}

interface ExpandedTabsNavProps {
  tabs: ExpandedNavTab[];
  ariaLabel: string;
  className?: string;
}

export function ExpandedTabsNav({
  tabs,
  ariaLabel,
  className,
}: ExpandedTabsNavProps) {
  const [selected, setSelected] = useState<number | null>(0);
  const containerRef = useRef<HTMLElement>(null);
  const pendingSectionRef = useRef<string | null>(null);
  const prefersReducedMotion = useReducedMotion() ?? false;
  const sectionKey = tabs
    .map((tab) => (tab.href.startsWith('#') ? tab.href.slice(1) : ''))
    .filter(Boolean)
    .join('|');

  useEffect(() => {
    const sectionIds = sectionKey.split('|').filter(Boolean);
    const sections = sectionIds
      .map((id) => document.getElementById(id))
      .filter((section): section is HTMLElement => section instanceof HTMLElement);
    if (sections.length === 0 || typeof IntersectionObserver === 'undefined') return;

    const headerOffset = 104;
    const updateActiveSection = () => {
      const activationLine = Math.max(headerOffset, window.innerHeight * 0.38);
      const pendingSectionId = pendingSectionRef.current;
      if (pendingSectionId) {
        const pendingIndex = sectionIds.indexOf(pendingSectionId);
        const pendingSection = sections.find(
          (section) => section.id === pendingSectionId,
        );
        if (pendingIndex !== -1 && pendingSection) {
          const bounds = pendingSection.getBoundingClientRect();
          if (bounds.top <= activationLine && bounds.bottom > headerOffset) {
            pendingSectionRef.current = null;
          } else {
            setSelected(pendingIndex);
            return;
          }
        } else {
          pendingSectionRef.current = null;
        }
      }

      const activeSection = sections
        .map((section) => {
          const bounds = section.getBoundingClientRect();
          return { section, top: bounds.top, bottom: bounds.bottom };
        })
        .filter(({ top, bottom }) => top <= activationLine && bottom > headerOffset)
        .sort((left, right) => right.top - left.top)[0]?.section;

      if (activeSection) {
        const activeIndex = sectionIds.indexOf(activeSection.id);
        if (activeIndex !== -1) setSelected(activeIndex);
      }
    };

    const cancelPendingNavigation = () => {
      if (!pendingSectionRef.current) return;
      pendingSectionRef.current = null;
      window.requestAnimationFrame(updateActiveSection);
    };

    const handlePointerDown = (event: PointerEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) {
        cancelPendingNavigation();
      }
    };

    const handleScrollKey = (event: KeyboardEvent) => {
      if (
        ['ArrowDown', 'ArrowUp', 'End', 'Home', 'PageDown', 'PageUp', ' '].includes(
          event.key,
        )
      ) {
        cancelPendingNavigation();
      }
    };

    const observer = new IntersectionObserver(updateActiveSection, {
      rootMargin: '-104px 0px -62% 0px',
      threshold: 0,
    });
    sections.forEach((section) => observer.observe(section));
    updateActiveSection();
    document.addEventListener('pointerdown', handlePointerDown);
    document.addEventListener('keydown', handleScrollKey);
    window.addEventListener('wheel', cancelPendingNavigation, { passive: true });
    window.addEventListener('touchstart', cancelPendingNavigation, { passive: true });
    window.addEventListener('resize', updateActiveSection, { passive: true });

    return () => {
      observer.disconnect();
      document.removeEventListener('pointerdown', handlePointerDown);
      document.removeEventListener('keydown', handleScrollKey);
      window.removeEventListener('wheel', cancelPendingNavigation);
      window.removeEventListener('touchstart', cancelPendingNavigation);
      window.removeEventListener('resize', updateActiveSection);
    };
  }, [sectionKey]);

  const labelVariants = {
    initial: { width: 0, opacity: 0 },
    animate: {
      width: 'auto',
      opacity: 1,
      transition: prefersReducedMotion
        ? { duration: 0 }
        : { delay: 0.05, duration: 0.2, ease: 'easeOut' as const },
    },
    exit: {
      width: 0,
      opacity: 0,
      transition: prefersReducedMotion
        ? { duration: 0 }
        : { duration: 0.1, ease: 'easeIn' as const },
    },
  };

  return (
    <nav
      ref={containerRef}
      aria-label={ariaLabel}
      className={cn(
        'items-center gap-1 rounded-full border border-border/60 bg-background/35 p-1 shadow-sm backdrop-blur-sm',
        className,
      )}
    >
      {tabs.map((tab, index) => {
        const Icon = tab.icon;
        const isSelected = selected === index;

        return (
          <a
            key={tab.href}
            href={tab.href}
            aria-label={tab.title}
            onClick={() => {
              pendingSectionRef.current = tab.href.startsWith('#')
                ? tab.href.slice(1)
                : null;
              setSelected(index);
            }}
            onFocus={() => setSelected(index)}
            className={cn(
              'relative z-10 inline-flex min-h-10 items-center justify-center rounded-full px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background motion-reduce:transition-none',
              isSelected
                ? 'text-foreground'
                : 'text-muted-foreground hover:bg-muted/70 hover:text-foreground',
            )}
          >
            {isSelected && (
              <motion.span
                aria-hidden="true"
                layoutId="public-header-tab-highlight"
                className="absolute inset-0 z-0 rounded-full border border-primary/20 bg-primary/10 shadow-sm"
                transition={
                  prefersReducedMotion
                    ? { duration: 0 }
                    : { type: 'spring', stiffness: 500, damping: 40 }
                }
              />
            )}

            <span className="relative z-10 flex items-center gap-2">
              <Icon aria-hidden="true" className="size-5 shrink-0" />
              <AnimatePresence initial={false}>
                {isSelected && (
                  <motion.span
                    variants={labelVariants}
                    initial="initial"
                    animate="animate"
                    exit="exit"
                    className="overflow-hidden whitespace-nowrap"
                  >
                    {tab.title}
                  </motion.span>
                )}
              </AnimatePresence>
            </span>
          </a>
        );
      })}
    </nav>
  );
}