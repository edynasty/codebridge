#ifndef CODEBRIDGE_PERMISSIONS_DARWIN_H
#define CODEBRIDGE_PERMISSIONS_DARWIN_H

// Caller frees the JSON result. No pixels, input content or filenames escape.
char *cb_daemon_permissions(int folders_only, const char *signing);

// Passive-only Computer-TCC posture for the daemon process. Never prompts, never requests.
// Caller frees the JSON result.
char *cb_daemon_passive_tcc(void);

#endif
