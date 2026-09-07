#define _GNU_SOURCE
#include <linux/seccomp.h>
#include <linux/filter.h>
#include <linux/audit.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <sched.h>
#include <stddef.h>
#include <errno.h>

#ifndef __NR_execveat
#define __NR_execveat 322
#endif
#ifndef __NR_process_vm_readv
#define __NR_process_vm_readv 310
#endif
#ifndef __NR_process_vm_writev
#define __NR_process_vm_writev 311
#endif
#ifndef __NR_setns
#define __NR_setns 308
#endif
#ifndef __NR_finit_module
#define __NR_finit_module 313
#endif
#ifndef __NR_perf_event_open
#define __NR_perf_event_open 298
#endif
#ifndef __NR_clock_adjtime
#define __NR_clock_adjtime 305
#endif
#ifndef __NR_kexec_load
#define __NR_kexec_load 246
#endif
#ifndef __NR_kexec_file_load
#define __NR_kexec_file_load 320
#endif
#ifndef __NR_bpf
#define __NR_bpf 321
#endif
#ifndef __NR_add_key
#define __NR_add_key 248
#endif
#ifndef __NR_request_key
#define __NR_request_key 249
#endif
#ifndef __NR_keyctl
#define __NR_keyctl 250
#endif
#ifndef __NR_kill
#define __NR_kill 62
#endif
#ifndef __NR_tkill
#define __NR_tkill 200
#endif
#ifndef __NR_tgkill
#define __NR_tgkill 234
#endif
#ifndef __NR_clone3
#define __NR_clone3 435
#endif
#ifndef CLONE_THREAD
#define CLONE_THREAD 0x00010000
#endif

/*
 * Denylist - syscalls with no legitimate use in a plugin context, grouped by
 * category: exec / process debug / mount / namespace / kernel modules /
 * bpf+perf / hw I/O / swap / system control / kexec / setuid family /
 * capabilities / keyring.
 *
 * Pattern: the DENY(nr) macro emits a self-contained 2-instruction block -
 * "if syscall == nr return EPERM, else skip to the next check". There are NO
 * hand-computed jump offsets, so adding or removing a syscall is a one-line
 * change that cannot desync the filter. Anything not denied falls through to
 * the final RET_ALLOW.
 */

/* DENY(nr): match → fall through to the RET_ERRNO right after; no match →
 * jump over it (jf=1) to the next check. */
#define DENY(nr) \
    BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, (nr), 0, 1), \
    BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ERRNO | (EPERM & SECCOMP_RET_DATA))

static struct sock_filter filter[] = {
    BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(struct seccomp_data, nr)),

    /* Signals targeting PID/TID 1 (the server runs as PID 1 in the container).
     * A plugin shares the server's UID, so it could otherwise kill(1)/tkill(1)/
     * tgkill(1,...) to DoS it. We deny tkill/tgkill ONLY when args[0]==1, and
     * deny kill outright below - this still allows raise()/abort() and pthread
     * self-signaling (a plugin's own tid is never 1, since it is a child PID).
     * Offsets are local to this 6-instruction prelude and never need to change
     * when the denylist below grows. */
    BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_tkill,  1, 0),                       /* [1] tkill?  → [3] */
    BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_tgkill, 0, 3),                       /* [2] tgkill? → [3]; else → [6] */
    BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(struct seccomp_data, args[0])),   /* [3] A = target (low 32) */
    BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, 1, 0, 1),                                 /* [4] ==1? → [5]; else → [6] */
    BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ERRNO | (EPERM & SECCOMP_RET_DATA)),    /* [5] deny */
    BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(struct seccomp_data, nr)),        /* [6] reload nr */

    /* exec */
    DENY(__NR_execve),
    DENY(__NR_execveat),

    /* process control / debug - trace or read another process's memory */
    DENY(__NR_ptrace),
    DENY(__NR_process_vm_readv),
    DENY(__NR_process_vm_writev),

    /* process signals - kill (any target) has no legitimate plugin use; tkill/
     * tgkill to PID 1 are already denied by the prelude above. */
    DENY(__NR_kill),

    /* filesystem mount */
    DENY(__NR_mount),
    DENY(__NR_umount2),
    DENY(__NR_pivot_root),
    DENY(__NR_chroot),

    /* namespaces */
    DENY(__NR_unshare),
    DENY(__NR_setns),

    /* kernel modules */
    DENY(__NR_init_module),
    DENY(__NR_finit_module),
    DENY(__NR_delete_module),

    /* BPF / perf */
    DENY(__NR_bpf),
    DENY(__NR_perf_event_open),

    /* io_uring / userfaultfd - premier kernel-exploit primitives (long LPE CVE
       history; ChromeOS/Android/GKE disable io_uring for this). No plugin runtime
       needs them by default. Guarded: older build headers may lack the numbers. */
#ifdef __NR_io_uring_setup
    DENY(__NR_io_uring_setup),
#endif
#ifdef __NR_io_uring_enter
    DENY(__NR_io_uring_enter),
#endif
#ifdef __NR_io_uring_register
    DENY(__NR_io_uring_register),
#endif
#ifdef __NR_userfaultfd
    DENY(__NR_userfaultfd),
#endif

    /* hardware I/O - x86-only syscalls (I/O port space); they do not exist on
       arm64 and other portless arches, so guard them or the build won't compile
       there (there is no such syscall to deny in the first place). */
#ifdef __NR_ioperm
    DENY(__NR_ioperm),
#endif
#ifdef __NR_iopl
    DENY(__NR_iopl),
#endif

    /* swap / memory mgmt */
    DENY(__NR_swapon),
    DENY(__NR_swapoff),

    /* system control */
    DENY(__NR_syslog),
    DENY(__NR_reboot),
    DENY(__NR_settimeofday),
    DENY(__NR_clock_settime),
    DENY(__NR_clock_adjtime),
    DENY(__NR_adjtimex),

    /* kexec */
    DENY(__NR_kexec_load),
    DENY(__NR_kexec_file_load),

    /* setuid family (escalation forbidden) */
    DENY(__NR_setuid),
    DENY(__NR_setgid),
    DENY(__NR_setreuid),
    DENY(__NR_setregid),
    DENY(__NR_setresuid),
    DENY(__NR_setresgid),

    /* capabilities */
    DENY(__NR_capset),

    /* keyring */
    DENY(__NR_add_key),
    DENY(__NR_request_key),
    DENY(__NR_keyctl),

    /* Process creation - LAST, because the clone block below loads args[0] into
     * A and every DENY above needs A to still hold the syscall number.
     *
     * This library is preloaded ONLY into plugins that did not ask for the
     * "subprocess" permission, and execve is already denied for them above:
     * forking could therefore never do anything but duplicate the plugin
     * itself, which is the fork-bomb primitive and nothing else. Threads stay
     * allowed, so node/python/Go runtimes are unaffected.
     *
     * clone3 gets ENOSYS, not EPERM: seccomp cannot dereference the struct
     * clone_args pointer it takes, so its flags are invisible here. ENOSYS
     * makes glibc >= 2.34 fall back to clone(2) - which we CAN inspect - and
     * pthread_create keeps working. EPERM would break every threaded runtime. */
    BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_clone3, 0, 1),
    BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ERRNO | (ENOSYS & SECCOMP_RET_DATA)),
#ifdef __NR_fork
    DENY(__NR_fork),
#endif
#ifdef __NR_vfork
    DENY(__NR_vfork),
#endif
    /* clone(2): the same syscall makes a thread or a process, told apart by
     * CLONE_THREAD in args[0]. Allow it set, deny it clear. */
    BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, __NR_clone, 0, 5),                        /* [0] not clone → ALLOW */
    BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(struct seccomp_data, args[0])),   /* [1] A = flags (low 32) */
    BPF_STMT(BPF_ALU | BPF_AND | BPF_K, CLONE_THREAD),                            /* [2] A &= CLONE_THREAD */
    BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, 0, 1, 0),                                 /* [3] clear? → [5] deny */
    BPF_STMT(BPF_JMP | BPF_JA, 1),                                                /* [4] thread → ALLOW */
    BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ERRNO | (EPERM & SECCOMP_RET_DATA)),    /* [5] deny */

    BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ALLOW),
};

static struct sock_fprog prog = {
    .len = sizeof(filter) / sizeof(filter[0]),
    .filter = filter,
};

__attribute__((constructor))
static void sandbox_init(void) {
    prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0);
    /*
     * PR_SET_DUMPABLE=0 makes /proc/PID/{environ,mem,maps,fd,...} owned by
     * root and unreadable by same-UID siblings. With seccomp already blocking
     * ptrace this is mainly about passive /proc reads - a co-resident plugin
     * cannot scrape another plugin's environment variables or memory maps
     * via the filesystem. Also disables coredumps (which could leak memory
     * to disk on a crash).
     */
    prctl(PR_SET_DUMPABLE, 0, 0, 0, 0);
    prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &prog);
}
