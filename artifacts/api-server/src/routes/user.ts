import { Router, type IRouter } from "express";
import { eq, sql } from "drizzle-orm";
import { clearCookie, deleteNativeSession } from "../lib/nativeAuth";
import {
  db,
  habitsTable,
  goalsTable,
  eventsTable,
  choresTable,
  notesTable,
  actionItemsTable,
  dailyPlansTable,
  gmailTokensTable,
  googleConnectionsTable,
  usersTable,
  sessionsTable,
  providerAccountsTable,
  authPasswordsTable,
  authEmailChallengesTable,
  authRecoveryMethodsTable,
  authTotpTable,
  authRecoveryCodesTable,
  authSecurityEventsTable,
} from "@workspace/db";

const router: IRouter = Router();

async function deleteAllUserData(userId: string) {
  // habit_completions cascades automatically when habits are deleted.
  await db.delete(habitsTable).where(eq(habitsTable.userId, userId));
  await db.delete(goalsTable).where(eq(goalsTable.userId, userId));
  await db.delete(eventsTable).where(eq(eventsTable.userId, userId));
  await db.delete(choresTable).where(eq(choresTable.userId, userId));
  await db.delete(notesTable).where(eq(notesTable.userId, userId));
  await db.delete(actionItemsTable).where(eq(actionItemsTable.userId, userId));
  await db.delete(dailyPlansTable).where(eq(dailyPlansTable.userId, userId));
  await db.delete(gmailTokensTable).where(eq(gmailTokensTable.userId, userId));
  await db.delete(googleConnectionsTable).where(eq(googleConnectionsTable.userId, userId));
}

async function deleteAuthenticationData(userId: string) {
  await db.delete(authSecurityEventsTable).where(eq(authSecurityEventsTable.userId, userId));
  await db.delete(authEmailChallengesTable).where(eq(authEmailChallengesTable.userId, userId));
  await db.delete(authRecoveryCodesTable).where(eq(authRecoveryCodesTable.userId, userId));
  await db.delete(authRecoveryMethodsTable).where(eq(authRecoveryMethodsTable.userId, userId));
  await db.delete(authTotpTable).where(eq(authTotpTable.userId, userId));
  await db.delete(authPasswordsTable).where(eq(authPasswordsTable.userId, userId));
  await db.delete(providerAccountsTable).where(eq(providerAccountsTable.userId, userId));
  await db.delete(sessionsTable).where(sql`${sessionsTable.sess}->>'userId' = ${userId}`);
}

// DELETE /user/data
// Wipes all user-created records while keeping the native authentication account.
router.delete("/user/data", async (req, res): Promise<void> => {
  try {
    await deleteAllUserData(req.dbUser.id);
    res.sendStatus(204);
  } catch (err) {
    req.log.error(err, "User data deletion failed");
    res.status(500).json({ error: "Failed to delete user data" });
  }
});

// PATCH /user/profile
// Updates editable local profile fields without changing the login identity.
router.patch("/user/profile", async (req, res): Promise<void> => {
  const firstName = typeof req.body?.firstName === "string" ? req.body.firstName.trim() : "";
  const lastName = typeof req.body?.lastName === "string" ? req.body.lastName.trim() : "";

  if (firstName.length > 100 || lastName.length > 100) {
    res.status(400).json({ error: "Profile names are too long" });
    return;
  }

  try {
    const [updated] = await db
      .update(usersTable)
      .set({
        firstName: firstName || null,
        lastName: lastName || null,
        updatedAt: new Date(),
      })
      .where(eq(usersTable.id, req.dbUser.id))
      .returning();
    res.json({
      id: updated.id,
      email: updated.email,
      firstName: updated.firstName,
      lastName: updated.lastName,
      profileImageUrl: updated.profileImageUrl,
    });
  } catch (err) {
    req.log.error({ err, userId: req.dbUser.id }, "Profile update failed");
    res.status(500).json({ error: "Failed to update profile" });
  }
});

// DELETE /user/account
// Wipes all data, deletes the native account, and revokes the current session.
router.delete("/user/account", async (req, res): Promise<void> => {
  try {
    await deleteAllUserData(req.dbUser.id);
    await deleteAuthenticationData(req.dbUser.id);
    await db.delete(usersTable).where(eq(usersTable.id, req.dbUser.id));
    await deleteNativeSession(req.nativeSessionId);
    clearCookie(res, "sid");
    res.sendStatus(204);
  } catch (err) {
    req.log.error({ err, userId: req.dbUser.id }, "Native account deletion failed");
    res.status(500).json({ error: "Failed to delete account" });
  }
});

export default router;
