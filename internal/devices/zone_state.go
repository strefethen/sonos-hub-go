package devices

import "github.com/strefethen/sonos-hub-go/internal/sonos/soap"

func convertZoneGroupState(state soap.ZoneGroupState) *ZoneGroupTopology {
	if len(state.Groups) == 0 {
		return nil
	}

	groups := make([]ZoneGroup, 0, len(state.Groups))
	for _, group := range state.Groups {
		members := make([]ZoneMember, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, ZoneMember{
				UDN:           member.UUID,
				Location:      member.Location,
				ZoneName:      member.ZoneName,
				IsCoordinator: member.IsCoordinator,
				IsSatellite:   member.IsSatellite,
				IsSubwoofer:   member.IsSubwoofer,
				ChannelMapSet: member.ChannelMapSet,
			})
		}
		groups = append(groups, ZoneGroup{
			GroupID:        group.ID,
			CoordinatorUDN: group.Coordinator,
			Members:        members,
		})
	}

	return &ZoneGroupTopology{Groups: groups}
}
