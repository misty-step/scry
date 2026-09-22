export function fatalContainerError(error, log = console.error) {
  log("[scry-container] error:", String(error));
  throw error;
}
