/**
 * WebSocket routes for live log streaming.
 */

export default async function websocketRoutes(fastify, options) {
  const { state } = options;

  // WebSocket endpoint for live logs
  fastify.get('/logs', { websocket: true }, (connection, req) => {
    // Validate session
    const sessionId = req.cookies.session_id;
    if (!state.auth.validateSession(sessionId)) {
      connection.close(4001, 'Unauthorized');
      return;
    }
    
    // Add client to logger
    state.logger.addClient(connection);
    
    // Handle close
    connection.on('close', () => {
      state.logger.removeClient(connection);
    });
    
    // Handle errors
    connection.on('error', () => {
      state.logger.removeClient(connection);
    });
  });
}
