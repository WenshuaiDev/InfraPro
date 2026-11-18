// Fixture/operator scripts must stop at 202. Interactive enrollment belongs to
// the personnel client; pending credentials must never become a saved session.
export function requireAuthenticatedLogin(result) {
  if (result?.status !== 'authenticated' || !result.account?.id) {
    throw new Error('Login is incomplete. Complete MFA in the console; no authenticated fixture session was created.');
  }
  return result;
}
export async function requireAuthenticatedResponse(response) {
  if (response.status() !== 200) throw new Error('Login did not return a formal session. Complete MFA in the console.');
  requireAuthenticatedLogin(await response.json());
  return response;
}
