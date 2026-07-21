import { Router, type IRouter } from "express";
import { eq } from "drizzle-orm";
import { getAuth } from "@clerk/express";
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

// DELETE /user/data
// Wipes all user-created records but keeps the Clerk account.
router.delete("/user/data", async (req, res): Promise<void> => {
  try {
    await deleteAllUserData(req.dbUser.id);
    res.sendStatus(204);
  } catch (err) {
    req.log.error(err, "User data deletion failed");
    res.status(500).json({ error: "Failed to delete user data" });
  }
});

// DELETE /user/account
// Wipes all data AND deletes the Clerk user, then the local users row.
//
// Ordering rationale:
//  1. Delete the Clerk identity first (authoritative). If this fails we return
//     an error — the user can still sign in, nothing is lost.
//  2. If Clerk deletion succeeds, delete app data and the local users row.
//     A partial failure here leaves orphaned rows (acceptable; the Clerk
//     identity is already gone so no one can access them).
router.delete("/user/account", async (req, res): Promise<void> => {
  // auth.userId is always the native Clerk user ID.
  // req.dbUser.id may be a legacy remapped ID for pre-migration users.
  const auth = getAuth(req);
  const clerkUserId = auth.userId;

  if (!clerkUserId) {
    res.status(401).json({ error: "Unauthorized" });
    return;
  }

  const secretKey = process.env.CLERK_SECRET_KEY;
  if (!secretKey) {
    req.log.error("CLERK_SECRET_KEY is not set — cannot delete Clerk user");
    res.status(500).json({ error: "Account deletion is not configured" });
    return;
  }

  try {
    // Step 1 — Delete the Clerk identity (authoritative).
    const clerkRes = await fetch(`https://api.clerk.com/v1/users/${clerkUserId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${secretKey}` },
    });

    if (!clerkRes.ok) {
      const body = await clerkRes.text().catch(() => "");
      req.log.error(
        { status: clerkRes.status, clerkUserId, body },
        "Clerk user deletion failed",
      );
      res.status(500).json({ error: "Failed to delete account. Please try again." });
      return;
    }

    // Step 2 — Delete app data. Best-effort: Clerk is already gone so errors
    //           here produce orphaned rows that no one can access.
    try {
      await deleteAllUserData(req.dbUser.id);
      await db.delete(usersTable).where(eq(usersTable.id, req.dbUser.id));
    } catch (dataErr) {
      req.log.error(
        { err: dataErr, clerkUserId, dbUserId: req.dbUser.id },
        "App data cleanup failed after Clerk deletion (orphaned rows)",
      );
      // Still return 204 — the Clerk identity (the gating record) is gone.
    }

    res.sendStatus(204);
  } catch (err) {
    req.log.error(err, "User account deletion failed");
    res.status(500).json({ error: "Failed to delete account" });
  }
});

export default router;
