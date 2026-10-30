package gormstore

func filterPage[M any, T any](records []M, match func(M) bool, skip, limit int, toDomain func(M) *T) ([]*T, bool) {
	if match != nil {
		matched := records[:0]
		for _, record := range records {
			if match(record) {
				matched = append(matched, record)
			}
		}
		records = matched
		if skip >= len(records) {
			records = nil
		} else {
			records = records[skip:]
		}
	}

	hasNext := len(records) > limit
	if hasNext {
		records = records[:limit]
	}
	items := make([]*T, len(records))
	for i := range records {
		items[i] = toDomain(records[i])
	}
	return items, hasNext
}
