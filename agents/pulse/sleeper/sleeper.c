// The sleeper holds an agent's place between heartbeats (08 §1): it waits, then replaces itself with the agent again, so
// the Go runtime is out of memory for all but the moment of a beat. Linux only, no libc. `make pulse-sleeper` rebuilds it.
//
// argv: sleeper <milliseconds> <agent path>
#if defined(__x86_64__)
#define SYS_SIGACTION 13
#define SYS_SIGPROCMASK 14
#define SYS_NANOSLEEP 35
#define SYS_EXECVE 59
#define SYS_EXIT 231
#define SYS_PRCTL 157
static long sc(long n, long a, long b, long c, long d) {
	long r;
	register long r10 __asm__("r10") = d;
	__asm__ volatile("syscall" : "=a"(r) : "a"(n), "D"(a), "S"(b), "d"(c), "r"(r10) : "rcx", "r11", "memory");
	return r;
}
__asm__(".global _start\n_start:\n mov %rsp,%rdi\n and $-16,%rsp\n call cmain\n");
struct action { void (*handler)(int); unsigned long flags; void (*restorer)(int); unsigned long mask; };
#define ACTION(h) {h, 0x04000000UL, h, 0} // SA_RESTORER: the kernel wants one, and the handler never returns to it
#elif defined(__aarch64__)
#define SYS_SIGACTION 134
#define SYS_SIGPROCMASK 135
#define SYS_NANOSLEEP 101
#define SYS_EXECVE 221
#define SYS_EXIT 94
#define SYS_PRCTL 167
static long sc(long n, long a, long b, long c, long d) {
	register long x8 __asm__("x8") = n, x0 __asm__("x0") = a, x1 __asm__("x1") = b, x2 __asm__("x2") = c, x3 __asm__("x3") = d;
	__asm__ volatile("svc 0" : "+r"(x0) : "r"(x8), "r"(x1), "r"(x2), "r"(x3) : "memory");
	return x0;
}
__asm__(".global _start\n_start:\n mov x0,sp\n and sp,x0,#-16\n bl cmain\n");
struct action { void (*handler)(int); unsigned long flags; unsigned long mask; };
#define ACTION(h) {h, 0, 0}
#else
#error "unsupported architecture"
#endif

// As PID 1 in a container a signal without a handler does nothing, and `docker stop` would wait for its timeout.
static void stop(int s) {
	(void)s;
	sc(SYS_EXIT, 0, 0, 0, 0);
}

__attribute__((used)) void cmain(long *sp) {
	long argc = sp[0];
	char **argv = (char **)(sp + 1);
	char **envp = argv + argc + 1;
	if (argc < 3) sc(SYS_EXIT, 2, 0, 0, 0);
	sc(SYS_PRCTL, 15, (long)"hashi-pulse", 0, 0); // PR_SET_NAME: started from an unnamed file, ps and top would show a number
	struct action a = ACTION(stop);
	sc(SYS_SIGACTION, 15, (long)&a, 0, 8);
	sc(SYS_SIGACTION, 2, (long)&a, 0, 8);
	unsigned long none = 0;
	sc(SYS_SIGPROCMASK, 2, (long)&none, 0, 8); // SIG_SETMASK: nothing blocked, whatever the caller had
	long ms = 0;
	for (char *p = argv[1]; *p; p++) ms = ms * 10 + (*p - '0');
	long ts[2] = {ms / 1000, ms % 1000 * 1000000};
	sc(SYS_NANOSLEEP, (long)ts, 0, 0, 0);
	sc(SYS_EXECVE, (long)argv[2], (long)(argv + 2), (long)envp, 0);
	sc(SYS_EXIT, 1, 0, 0, 0); // the agent's file is gone: the service manager's restart takes over
}
