import { motion } from 'framer-motion';

const sloganTextClassName =
  'm-0 font-display text-2xl font-bold leading-tight tracking-tight text-foreground sm:text-3xl md:text-4xl';

function SloganWord({
  initial,
  rest,
  index,
  layoutIdPrefix,
  reduceMotion,
}: {
  initial: string;
  rest: string;
  index: number;
  layoutIdPrefix?: string;
  reduceMotion?: boolean;
}) {
  return (
    <span className="whitespace-nowrap">
      <span className="inline-grid align-baseline">
        <span aria-hidden="true" className="col-start-1 row-start-1 font-bold text-primary">
          {initial.toUpperCase()}
        </span>
        <motion.span
          layoutId={layoutIdPrefix ? `${layoutIdPrefix}-letter-${index}` : undefined}
          transition={{ duration: reduceMotion ? 0 : 0.75, ease: 'easeInOut' }}
          className="col-start-1 row-start-1 font-bold text-primary"
        >
          {initial.toUpperCase()}
        </motion.span>
      </span>
      {rest}
    </span>
  );
}

function SloganWords({
  layoutIdPrefix,
  reduceMotion,
}: {
  layoutIdPrefix?: string;
  reduceMotion?: boolean;
}) {
  return (
    <>
      <SloganWord initial="A" rest="ccelerate" index={0} layoutIdPrefix={layoutIdPrefix} reduceMotion={reduceMotion} />{' '}
      your progress,{' '}
      <SloganWord initial="S" rest="ynchronize" index={1} layoutIdPrefix={layoutIdPrefix} reduceMotion={reduceMotion} />{' '}
      your schedule,{' '}
      <SloganWord initial="K" rest="nit" index={2} layoutIdPrefix={layoutIdPrefix} reduceMotion={reduceMotion} />{' '}
      your day together—because you{' '}
      <SloganWord initial="o" rest="nly" index={3} layoutIdPrefix={layoutIdPrefix} reduceMotion={reduceMotion} />{' '}
      <SloganWord initial="l" rest="ive" index={4} layoutIdPrefix={layoutIdPrefix} reduceMotion={reduceMotion} />{' '}
      <SloganWord initial="o" rest="nce" index={5} layoutIdPrefix={layoutIdPrefix} reduceMotion={reduceMotion} />.
    </>
  );
}

export function BrandSloganFormation({
  layoutIdPrefix,
  reduceMotion,
}: {
  layoutIdPrefix: string;
  reduceMotion: boolean;
}) {
  return (
    <>
      <p aria-hidden="true" className={sloganTextClassName}>
        <SloganWords layoutIdPrefix={layoutIdPrefix} reduceMotion={reduceMotion} />
      </p>
      <p className="sr-only">
        Accelerate your progress, Synchronize your schedule, Knit your day
        together—because you only live once.
      </p>
    </>
  );
}

export function BrandSlogan() {
  return (
    <>
      <p aria-hidden="true" className={sloganTextClassName}>
        <SloganWords />
      </p>
      <p className="sr-only">
        Accelerate your progress, Synchronize your schedule, Knit your day
        together—because you only live once.
      </p>
    </>
  );
}