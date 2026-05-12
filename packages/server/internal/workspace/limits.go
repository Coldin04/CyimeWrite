package workspace

const (
	defaultFileListLimit = 50
	maxFileListLimit     = 100

	maxFolderDescriptionBytes = 4 * 1024

	// maxWorkspaceStorageBytesPerUser bounds workspace-owned text that is stored
	// in the database so one authenticated account cannot exhaust shared storage.
	maxWorkspaceStorageBytesPerUser = 50 * 1024 * 1024
)

func normalizePagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultFileListLimit
	}
	if limit > maxFileListLimit {
		limit = maxFileListLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
