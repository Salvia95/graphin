export function validate(req) {
  if (!req) {
    throw new Error("invalid request");
  }
  return true;
}
