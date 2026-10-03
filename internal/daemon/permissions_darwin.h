#ifndef CODEBRIDGE_PERMISSIONS_DARWIN_H
#define CODEBRIDGE_PERMISSIONS_DARWIN_H

// Caller frees the JSON result. No pixels, input content or filenames escape.
char *cb_daemon_permissions(int folders_only, const char *signing);

#endif
