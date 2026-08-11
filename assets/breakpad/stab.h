/*
 * Breakpad's STABS reader includes the GNU header unconditionally. Alpine's
 * musl development headers do not ship it, but the reader only needs these
 * format tags for ELF input.
 */
#ifndef L4D2_PANEL_BREAKPAD_STAB_H
#define L4D2_PANEL_BREAKPAD_STAB_H

#ifndef N_UNDF
#define N_UNDF 0x00
#endif
#ifndef N_FUN
#define N_FUN 0x24
#endif
#ifndef N_SLINE
#define N_SLINE 0x44
#endif
#ifndef N_SO
#define N_SO 0x64
#endif
#ifndef N_SOL
#define N_SOL 0x84
#endif

#endif
