function SloganWord({ initial, rest }: { initial: string; rest: string }) {
  return (
    <span className="whitespace-nowrap">
      <span className="font-semibold text-primary">{initial.toUpperCase()}</span>
      {rest}
    </span>
  );
}

export function BrandSlogan() {
  return (
    <div className="mx-auto max-w-3xl">
      <p
        aria-hidden="true"
        className="text-base leading-relaxed text-muted-foreground sm:text-lg"
      >
        <SloganWord initial="A" rest="ccelerate" /> your progress,{' '}
        <SloganWord initial="S" rest="ynchronize" /> your schedule,{' '}
        <SloganWord initial="K" rest="nit" /> your day together—because you{' '}
        <SloganWord initial="o" rest="nly" /> <SloganWord initial="l" rest="ive" />{' '}
        <SloganWord initial="o" rest="nce" />.
      </p>
      <p className="sr-only">
        Accelerate your progress, Synchronize your schedule, Knit your day
        together—because you only live once.
      </p>
    </div>
  );
}