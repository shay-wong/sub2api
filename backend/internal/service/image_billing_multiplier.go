package service

func resolveImageRateMultiplierForGroup(group *Group, effectiveGroupMultiplier float64) float64 {
	if group != nil && group.ImageRateIndependent {
		if group.ImageRateMultiplier < 0 {
			return 0
		}
		return group.ImageRateMultiplier
	}
	return effectiveGroupMultiplier
}
