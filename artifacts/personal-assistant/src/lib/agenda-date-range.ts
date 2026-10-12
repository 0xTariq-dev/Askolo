import { addDays, format, startOfMonth, startOfWeek } from 'date-fns';

const MONTH_GRID_DAYS = 42;

export function getMonthGridAgendaRange(
  monthDate: Date,
  weekStartsOn: 0 | 1 | 2 | 3 | 4 | 5 | 6,
): { from: string; to: string } {
  const firstVisibleDate = startOfWeek(startOfMonth(monthDate), { weekStartsOn });
  return {
    from: format(firstVisibleDate, 'yyyy-MM-dd'),
    to: format(addDays(firstVisibleDate, MONTH_GRID_DAYS - 1), 'yyyy-MM-dd'),
  };
}
