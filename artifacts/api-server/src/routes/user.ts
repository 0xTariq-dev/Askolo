import { Router, type IRouter } from "express";
import { eq } from "drizzle-orm";
import { getAuth } from "@clerk/express";
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
// Wipes all data AND deletes the Clerk user, then the local users row.
//
// Ordering rationale:
//  1. Delete the Clerk identity first (authoritative). If this fails we return
//     an error — the user can still sign in, nothing is lost.
//  2. If Clerk deletion succeeds, delete app data and the local users row.
//     A partial failure here leaves orphaned rows (acceptable; the Clerk
//     identity is already gone so no one can access them).
router.delete("/user/account", async (req, res): Promise<void> => {
  if (req.authProvider === "google") {
    try {
      await deleteAllUserData(req.dbUser.id);
      await db.delete(usersTable).where(eq(usersTable.id, req.dbUser.id));
      await deleteNativeSession(req.nativeSessionId);
      clearCookie(res, "sid");
      res.sendStatus(204);
    } catch (err) {
      req.log.error({ err, userId: req.dbUser.id }, "Native account deletion failed");
      res.status(500).json({ error: "Failed to delete account" });
    }
    return;
  }

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
