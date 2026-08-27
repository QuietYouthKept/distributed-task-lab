package domain

type DBPoolStats struct {
	OpenConnections     int
	InUse               int
	Idle                int
	WaitCount           int64
	WaitDurationSeconds float64
}
