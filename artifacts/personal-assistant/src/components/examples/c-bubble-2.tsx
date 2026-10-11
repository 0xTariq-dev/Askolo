import { Bubble, BubbleContent } from "@/components/ui/bubble"

// `as const` narrows every entry to the literal "secondary" / "start" types the
// Bubble unions expect, so the map below needs no type import and no cast.
const turns = [
  {
    id: "t1",
    variant: "secondary",
    align: "start",
    text: "Deploy 3.4.2 is queued behind two migrations.",
  },
  {
    id: "t2",
    variant: "default",
    align: "end",
    text: "Go ahead. I will watch the error rate.",
  },
  {
    id: "t3",
    variant: "muted",
    align: "start",
    text: "Migration 2 of 2 finished in 41s.",
  },
  // The destructive turn names the failure in its own words. A variant that
  // signals an error only by colour fails every reader who cannot see it.
  {
    id: "t4",
    variant: "destructive",
    align: "start",
    text: "Step 5 failed. The events worker did not come back after the restart.",
  },
  {
    id: "t5",
    variant: "outline",
    align: "start",
    text: "stripe-webhook.ts returned 500 on charge.refunded, 41 events queued",
  },
  {
    id: "t6",
    variant: "tinted",
    align: "end",
    text: "Restart the worker and retry step 5.",
  },
  // ghost is the assistant rung: it drops the frame and the 80% cap, which is
  // why this turn is the only one reaching the full width of the row.
  {
    id: "t7",
    variant: "ghost",
    align: "start",
    text: "The worker came back and step 5 replayed all 41 queued events without a second failure. I have left the retry window at 60 seconds so a slow processor cannot drop another refund.",
  },
] as const

export function Pattern() {
  return (
    <div className="mx-auto flex w-full max-w-sm flex-col gap-3">
      {turns.map((turn) => (
        <Bubble key={turn.id} variant={turn.variant} align={turn.align}>
          {/* Nothing is set on BubbleContent, so each style resolves its own
              radius, padding and font size. One stray text-sm here would pin
              all eight to it. */}
          <BubbleContent>{turn.text}</BubbleContent>
        </Bubble>
      ))}
    </div>
  )
}