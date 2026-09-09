import { Client } from "appwrite";

const APPWRITE_ENDPOINT = "https://fra.cloud.appwrite.io/v1";
const APPWRITE_PROJECT_ID = "6aa13ce5002f4e84f140";

export const appwriteClient = new Client()
  .setEndpoint(APPWRITE_ENDPOINT)
  .setProject(APPWRITE_PROJECT_ID);

export function pingAppwrite(): void {
  void appwriteClient
    .ping()
    .then(() => {
      console.info("[Appwrite] Server ping succeeded.");
    })
    .catch((error: unknown) => {
      console.error("[Appwrite] Server ping failed.", error);
    });
}