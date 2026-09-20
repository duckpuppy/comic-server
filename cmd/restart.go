package cmd

import "sync/atomic"

// restartRequested is set by runServer when it is returning because the API
// asked for a restart (POST /api/system/restart, comic-server-9klu) rather
// than because of a signal or an error. Execute checks it AFTER
// rootCmd.Execute has returned - i.e. after runServer's deferred cleanups
// (backend flush/close, config.db close, listener shutdown) have all run -
// and only then re-execs the binary in place (see execSelf in
// restart_unix.go). Exec-ing from inside runServer or the HTTP handler
// would replace the process image without running any of those defers.
var restartRequested atomic.Bool
