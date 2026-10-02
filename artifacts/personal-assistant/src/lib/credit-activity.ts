import type { CreditEvent, CreditReservation, CreditUsageResponse } from './credit-api';

export type CreditActivity = CreditEvent | CreditReservation;

export function getConductedCreditActivities(
  usage: Pick<CreditUsageResponse, 'reservations' | 'events'>,
): CreditActivity[] {
  return [
    ...usage.reservations.filter((reservation) => reservation.status === 'settled'),
    ...usage.events.filter((event) => event.eventType === 'refunded'),
  ].sort((first, second) => Date.parse(second.createdAt) - Date.parse(first.createdAt));
}