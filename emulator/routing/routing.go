package routing

import (
	"fmt"
	"project/podman"
	"strconv"

	"github.com/rs/zerolog/log"
)

var (
	LINKS map[string]podman.LinkDetails
)

const printOn bool = false

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func max(a, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

func linkNameFromNodeId(node1, node2 int) (string, bool) {
	if node1 == node2 {
		panic("aaaaaaaaaaaaa!")
	}
	firstNode := min(node1, node2)
	secondNode := max(node1, node2)
	return "S" + strconv.Itoa(firstNode) + "-S" + strconv.Itoa(secondNode), firstNode != node1
}

// nodes = path.
func RouteTables(nodes []int) (map[int]string, map[int]string) { //nodes is path
	commands := make(map[int]string)
	reversecommands := make(map[int]string)
	linkid, ss := linkNameFromNodeId(nodes[len(nodes)-2], nodes[len(nodes)-1])
	linkDetails := LINKS[linkid]
	destinationIP := linkDetails.NodeOneIP // <--- destination should be Koto for forward routing

	if printOn {
		log.Info().Msg("\nROUTING\n")
	}

	if ss {
		destinationIP = linkDetails.NodeTwoIP
	}
	for i, node := range nodes { // Forward Routing
		if i == len(nodes)-2 {
			break
		}
		linkid2, swapped := linkNameFromNodeId(nodes[i], nodes[i+1])
		link := LINKS[linkid2]
		// fmt.Println(link)
		nexthopIP := link.NodeOneIP
		thisIP := link.NodeTwoIP
		if swapped {
			nexthopIP = link.NodeTwoIP
			thisIP = link.NodeOneIP
		}
		if printOn {
			log.Info().Int("SAT_ID_FROM", node).Str("THIS_IP", thisIP).Int("SAT_ID_TO", nodes[i+1]).Str("NEXT_IP", nexthopIP).Msg("PRIMARY FORWARD")
		}
		commands[node] = ipRouteVia(destinationIP, nexthopIP)
	}
	// 0  1   2   3]
	//[0, 63, 67, 94]
	linkid3, s := linkNameFromNodeId(nodes[0], nodes[1])
	link := LINKS[linkid3]
	destinationIP = link.NodeTwoIP // <--- destination should be ElAlamo for reverse routing
	if s {
		destinationIP = link.NodeOneIP
	}
	for i := len(nodes) - 1; i >= 0; i-- {
		if i <= 1 {
			break
		}
		linkid4, swp := linkNameFromNodeId(nodes[i-1], nodes[i])
		link := LINKS[linkid4]
		// fmt.Println(link)

		nexthopIP := link.NodeTwoIP
		thisIP := link.NodeOneIP
		if swp {
			nexthopIP = link.NodeOneIP
			thisIP = link.NodeTwoIP
		}
		if printOn {
			log.Info().Int("SAT_ID_FROM", nodes[i]).Str("THIS_IP", thisIP).Int("SAT_ID_TO", nodes[i-1]).Str("NEXT_IP", nexthopIP).Msg("PRIMARY REVERSE")
		}
		reversecommands[nodes[i]] = ipRouteVia(destinationIP, nexthopIP)
	}
	if printOn {
		log.Info().Interface("cmds", commands).Msg("FORWARD Routing PRIMARY")
		log.Info().Interface("cmds", reversecommands).Msg("REVERSE Routing PRIMARY")
	}

	return commands, reversecommands
}

func RouteTablesPrevSats(path, prevSats, prevSatsL2Path []int) (map[int]string, map[int]string) {
	commands := make(map[int]string)
	reversecommands := make(map[int]string)
	linkid, ss := linkNameFromNodeId(path[len(path)-2], path[len(path)-1])
	linkDetails := LINKS[linkid]
	destinationIP := linkDetails.NodeOneIP // <--- destination should be Koto for forward routing

	if ss {
		destinationIP = linkDetails.NodeTwoIP
	}
	for _, sati := range prevSats { // Forward Routing
		for j, satj := range prevSatsL2Path {
			// only route packets away from prevSats satellites
			if sati == satj {
				linkid2, swapped := linkNameFromNodeId(prevSatsL2Path[j], prevSatsL2Path[j+1])
				link := LINKS[linkid2]
				nexthopIP := link.NodeOneIP
				thisIP := link.NodeTwoIP
				if swapped {
					nexthopIP = link.NodeTwoIP
					thisIP = link.NodeOneIP
				}
				if printOn {
					log.Info().Int("SAT_ID_FROM", satj).Str("THIS_IP", thisIP).Int("SAT_ID_TO", prevSatsL2Path[j+1]).Str("NEXT_IP", nexthopIP).Msg("EXTRA FORWARD")
				}
				commands[satj] = ipRouteVia(destinationIP, nexthopIP)
			}
		}
	}

	linkid3, s := linkNameFromNodeId(path[0], path[1])
	link := LINKS[linkid3]
	destinationIP = link.NodeTwoIP // <--- destination should be ElAlamo for reverse routing
	if s {
		destinationIP = link.NodeOneIP
	}
	for _, sati := range prevSats { // Reverse Routing
		for j, satj := range prevSatsL2Path {
			// only route packets away from prevSats satellites
			if sati == satj {
				linkid4, swp := linkNameFromNodeId(prevSatsL2Path[j-1], prevSatsL2Path[j])
				link := LINKS[linkid4]
				nexthopIP := link.NodeTwoIP
				thisIP := link.NodeOneIP
				if swp {
					nexthopIP = link.NodeOneIP
					thisIP = link.NodeTwoIP
				}
				if printOn {
					log.Info().Int("SAT_ID_FROM", satj).Str("THIS_IP", thisIP).Int("SAT_ID_TO", prevSatsL2Path[j-1]).Str("THIS_IP", thisIP).Str("NEXT_IP", nexthopIP).Msg("EXTRA REVERSE")
				}
				reversecommands[satj] = ipRouteVia(destinationIP, nexthopIP)
			}
		}
	}

	if printOn {
		log.Info().Interface("cmds", commands).Msg("FORWARD Routing EXTRA")
		log.Info().Interface("cmds", reversecommands).Msg("REVERSE Routing EXTRA")

		log.Info().Msg("\nROUTING END\n")
	}

	return commands, reversecommands
}

func ipRouteVia(destinationIP, nexthopIP string) string {
	return fmt.Sprintf("ip route replace %s via %s", destinationIP, nexthopIP)
}
