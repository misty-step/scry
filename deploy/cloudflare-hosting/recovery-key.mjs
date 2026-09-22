const SNAPSHOT_KEY = /^scry-\d{8}T\d{6}\.\d{9}Z-[a-f0-9]{32}\.scry-backup\.zip$/;

export async function latestSnapshotKey(recovery) {
  if (!recovery || typeof recovery.list !== "function") {
    throw new Error("RECOVERY binding is required for restore-required boot");
  }
  let cursor;
  let latest = "";
  for (let page = 0; page < 100; page++) {
    const listing = await recovery.list({ cursor, limit: 1000 });
    if (!listing || !Array.isArray(listing.objects)) {
      throw new Error("RECOVERY listing returned an invalid response");
    }
    for (const item of listing.objects) {
      if (item && SNAPSHOT_KEY.test(item.key) && item.key > latest) latest = item.key;
    }
    if (!listing.truncated) {
      if (!latest) throw new Error("no complete recovery snapshot is available");
      return latest;
    }
    if (!listing.cursor) throw new Error("RECOVERY listing truncated without a cursor");
    cursor = listing.cursor;
  }
  throw new Error("RECOVERY listing exceeded its bounded page limit");
}
