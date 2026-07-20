import { verifyGoogleCalendarConnection } from "./googleCalendar";
import { verifyGmailConnection } from "./gmail";

export interface GoogleConnectionStatus {
  connected: boolean;
  scopes: string[];
  calendarConnected: boolean;
  gmailConnected: boolean;
}

export async function getGoogleConnectionStatus(): Promise<GoogleConnectionStatus> {
  const [calendarConnected, gmailConnected] = await Promise.all([
    verifyGoogleCalendarConnection(),
    verifyGmailConnection(),
  ]);
  const scopes = [
    ...(calendarConnected ? ["calendar"] : []),
    ...(gmailConnected ? ["gmail"] : []),
  ];
  return {
    connected: calendarConnected || gmailConnected,
    scopes,
    calendarConnected,
    gmailConnected,
  };
}
