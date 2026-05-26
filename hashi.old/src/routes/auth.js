/**
 * Authentication routes.
 */

export default async function authRoutes(fastify, options) {
  const { state } = options;

  // GET /auth/status - Get auth status
  fastify.get('/status', async (request, reply) => {
    const status = state.auth.getStatus();
    const sessionId = request.cookies.session_id;
    const loggedIn = state.auth.validateSession(sessionId);
    
    return { ...status, loggedIn };
  });

  // POST /auth/register - Register new user
  fastify.post('/register', async (request, reply) => {
    const { username, password } = request.body || {};
    
    const result = await state.auth.register(username, password);
    
    if (!result.success) {
      return reply.code(400).send(result);
    }
    
    return result;
  });

  // POST /auth/login - Login
  fastify.post('/login', async (request, reply) => {
    const { username, password } = request.body || {};
    
    const result = await state.auth.login(username, password);
    
    if (!result.success) {
      return reply.code(401).send(result);
    }
    
    // Set session cookie
    reply.setCookie('session_id', result.sessionId, {
      path: '/',
      httpOnly: true,
      sameSite: 'strict',
      secure: process.env.NODE_ENV === 'production',
      maxAge: 24 * 60 * 60, // 24 hours
    });
    
    return { success: true };
  });

  // POST /auth/logout - Logout
  fastify.post('/logout', async (request, reply) => {
    const sessionId = request.cookies.session_id;
    state.auth.logout(sessionId);
    
    reply.clearCookie('session_id', { path: '/' });
    return { success: true };
  });
}
