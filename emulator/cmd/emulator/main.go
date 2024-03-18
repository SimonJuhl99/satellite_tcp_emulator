package main

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"project/database"
	"project/graph"
	"project/linkset"
	"project/podman"
	"project/routing"
	"project/space"
	"project/tle"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joshuaferrara/go-satellite"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"
)

const maxFSODistance float64 = 3000
const oneweb_altitude = 1200

// const starlink_altitude = 550

// var maxFSODistance = space.LineOfSight(oneweb_altitude)

// var maxFSODistance = 2000.0

// distance =

var SatelliteIds []int
var GroundStations []space.GroundStation

func SetupLogger() *os.File {
	// creating a console
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr}

	// temporary file using default dir and (i think) the name including the time of the file creation
	tempFile, err := os.CreateTemp(os.TempDir(), "deleteme"+time.Now().Format(time.Kitchen))
	if err != nil {
		// Can we log an error before we have our logger? :)
		log.Error().Err(err).Msg("there was an error creating a temporary file for our log")
	}
	fmt.Printf("The log file is allocated at %s\n", tempFile.Name())

	// both write log message in console and file
	multi := zerolog.MultiLevelWriter(consoleWriter, tempFile)
	// configure logger time to unix timestamps (in ms)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	// create logger instance using the multi level writer
	log.Logger = zerolog.New(multi).With().Timestamp().Logger()
	return tempFile
}

func main() {
	constellation_name := "OneWeb"

	tempFile := SetupLogger()
	defer tempFile.Sync()
	defer tempFile.Close()

	log.Info().Float64("FSO Distance", maxFSODistance).Msg("Maximum Free Space Optical Distance")
	//* GETTING SAT DATA *//
	var err error

	// returns a slice containing instances of GroundStation struct
	GroundStations, err = space.LoadGroundStations("./groundstations.txt")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load groundstations")
	}
	log.Info().Int("groundStationCount", len(GroundStations)).Msg("loaded groundstations")

	// SatelliteIds, satellites, found := tle.LoadSatellites("./TD")
	// SatelliteIds, satellites, found := tle.LoadSatellites("./TD_full")
	var graphSize int = len(GroundStations)
	// var SatelliteIds []int

	// create slice of OrbitalData structs
	var satdata []space.OrbitalData
	startTime := time.Date(2022, 9, 11, 12, 00, 00, 00, time.UTC)
	endTime := time.Date(2022, 9, 21, 22, 00, 00, 00, time.UTC)
	timeStep := 15 * time.Second
	duration := endTime.Sub(startTime)
	// retrieve data generated in satellite_positions.py (based on Israels simulation)
	var satellite_positions_path string = "./constellation.parquet"
	if strings.ToLower(os.Getenv("ISRAEL")) == "true" { // <= QUESTION: Do you have an environment variable set controlling this decision?
		log.Info().Msg("using simulated constellation")
		// returns slice of OrbitalData structs, each struct containing positions (LatLong in degrees) for one satellite over time
		satdata = database.LoadSatellitePositions(satellite_positions_path, constellation_name, startTime, timeStep)
		graphSize += len(satdata)
		for _, orbitialData := range satdata {
			SatelliteIds = append(SatelliteIds, orbitialData.SatelliteId)
		}

	} else {
		log.Info().Msg("using propagated constellation")
		var satellites []satellite.Satellite
		var found bool
		// Creates slice of satellite structs using "https://github.com/joshuaferrara/go-satellite"
		SatelliteIds, satellites, found = tle.LoadSatellites("./OneWeb") // <= QUESTION: What are those informations in the file? And how does the satellite struct work?
		if !found {
			log.Fatal().Int("satelliteCount", len(SatelliteIds)).Msg("Failed to load satellites")
		}

		graphSize += len(SatelliteIds)
		// returns slice of OrbitalData structs, each struct containing positions (LatLong in degrees) for one satellite over time
		satdata = space.GetSatData(satellites, SatelliteIds, startTime, timeStep, duration)
	}
	log.Info().Int("satelliteCount", len(SatelliteIds)).Msg("Found satellites")

	sort.Ints(SatelliteIds) //satdata is sorted in GetSatData. SatelliteIds must be sorted to be used as common indexing

	// for _, gs := range GroundStations {
	// 	gs_positions := groundstation.GroundStationECIPostions(gs, startTime, timeStep, duration)
	// 	gs.Position = gs_positions
	// } // Currently Unused

	if os.Getenv("LOG_DATA") == "TRUE" {
		var location string = "parquet" // TODO: remove hard coding
		if location == "questdb" {
			work_channel := make(chan database.SatelliteLineData)
			// connect to QuestDB | work_channel is used to write to db
			go database.WriteWorker(work_channel, "")
			// for each satellite in satdata
			for _, satellitedata := range satdata {
				log.Info().Str("sattelliteid", satellitedata.Title).Msg("Processing Satellite")
				logtime := startTime
				// for each timestep (number of positions)
				for i := range satellitedata.Position {
					timestamp := logtime.UnixNano()
					// write to database
					work_channel <- database.SatelliteLineData{
						SatelliteID: satellitedata.SatelliteId,
						Title:       satellitedata.Title,
						Position:    satellitedata.Position[i],
						Velocity:    satellitedata.Velocity[i],
						LatLong:     satellitedata.LatLong[i],
						Timestamp:   timestamp,
						Index:       uint(i),
					}
					logtime = logtime.Add(1 * timeStep)
				}
			}
			close(work_channel)
			return
		} else if location == "parquet" {
			// returns parquet writer which take in a FlatSatelliteLineData struct and writes it to a file
			pw, stop := database.WriteLogs("satdata_complete", new(database.FlatSatelliteLineData))
			defer stop()
			// for each OrbitalData struct (each struct containing positions over time for one satellite)
			for _, satellitedata := range satdata {
				log.Info().Int("sattelliteid", satellitedata.SatelliteId).Msg("Processing Satellite")
				logtime := startTime
				// for each timestep (number of positions)
				for i := range satellitedata.Position {
					timestamp := logtime.UnixMilli()
					// prepare data for
					line_data := database.FlatSatelliteLineData{
						SatelliteID: int32(satellitedata.SatelliteId),
						PosX:        satellitedata.Position[i].X,
						PosY:        satellitedata.Position[i].Y,
						PosZ:        satellitedata.Position[i].Z,
						VelX:        satellitedata.Velocity[i].X,
						VelY:        satellitedata.Velocity[i].Y,
						VelZ:        satellitedata.Velocity[i].Z,
						Lattitude:   satellitedata.LatLong[i].Latitude,
						Longitude:   satellitedata.LatLong[i].Longitude,
						Timestamp:   timestamp,
						Index:       int32(i),
					}
					// write struct to parquet file
					err := pw.Write(line_data)
					if err != nil {
						log.Fatal().Err(err).Msg("failed writing to parquet")
					}

					logtime = logtime.Add(1 * timeStep)
				}
			}

			return
		}
	}

	// if os.Getenv("PARQUET_DATA") == "TRUE" {
	// 	pqWriter, stopfunc := WriteLogs("satdata", new(space.OrbitalData))
	// 	for _, satellitedata := range satdata {
	// 		if err = pqWriter.Write(satellitedata); err != nil {
	// 			panic(err)
	// 		}
	// 		pqWriter.Flush(true)
	// 	}
	// 	stopfunc()

	// }

	// timedata := make([]time.Time, duration/timeStep)
	// timedata[0] = startTime
	// for i := 1; i < int(duration)/int(timeStep); i++ {
	// 	timedata[i] = startTime.Add(timeStep)
	// }

	// make channel with operating system signal
	interruptSignal := make(chan os.Signal, 1)
	// relay Ctrl+C interrupt signal to the channel just created
	signal.Notify(interruptSignal, syscall.SIGINT)

	//* STARTING PODMAN CONTAINERS *//
	podman.InitPodman()
	podman.Cleanup()
	defer podman.Cleanup()
	containers := make([]string, len(SatelliteIds)+len(GroundStations))
	// wait for all the goroutines launched here to finish
	wg := sync.WaitGroup{}
	// for each satellite
	for i, satelliteid := range SatelliteIds {
		// increment wg counter (when it reaches 0 the group is no longer blocked)
		wg.Add(1)
		// a goroutine is launched for each container creation, when done it will signal wg that it is done (decrement counter)
		go func(index int, id int) {
			defer wg.Done()
			containers[index] = podman.CreateRunContainer("Sat"+strconv.Itoa(id), false, podman.SatelliteRawImage)
		}(i, satelliteid) // ¯\_(ツ)_/¯
	}
	for i, gs := range GroundStations {
		wg.Add(1)
		go func(index int, gs_id string) {
			defer wg.Done()
			containers[index] = podman.CreateRunContainer("GS"+gs_id, true, podman.GroundStationRawImage)
		}(len(satdata)+i, gs.Title)
	}
	// Block until the wg counter goes back to 0 (all containers have been created)
	wg.Wait()
	// Make a map of all links and a subnet they can use to easy setup a link later
	connections := AllConnections(&GroundStations) // slice of connection structs
	links := setupLinkMap(containers, satdata, GroundStations, connections)
	log.Info().Msg("created links") //.Interface("links", links)
	//* GRAPH *//
	log.Debug().Int("graphSize", len(SatelliteIds)).Msg("Size of Graph")
	// create graph's vertices (ground stations and sats)
	gn := graph.InstantiateGraph(graphSize) // T - nu

	var APRange float64 = 8.0 // km																								// QUESTION: Isn't 8km very little?
	graph.SetupGraphAccessPointEdges(gn, graphSize, GroundStations, APRange)
	log.Info().Float64("accessPointRange", APRange).Msg("graphAccessPointEdges")
	var activelinks []string
	var nextlinks []string
	// var prevlinks []string½
	// var substep int = 15 // 15/15s=1Hz
	var path, nextPath []int
	var pathDistance, nextPathDistance int64
	var newPath bool = false
	simulationStart := time.Now()
	log.Info().Time("simulationStart", simulationStart).Msg("starting simulation")

	for index := 0; index < (int(duration)/(int(timeStep)))-1; index++ {
		select {
		case stopsignal := <-interruptSignal:
			log.Info().Interface("signal", stopsignal).Msg("shutting down")
			time.Sleep(2 * time.Second)
			return
		default:
		}
		//ADD here if anything before new calc.
		//New graph,
		// if (index != 0) && (index%substep == 0) { // every major time step switch graph
		// 	gn = gt
		// }

		// gt = graph.InstantiateGraph(graphSize) // T - nu
		// graph.SetupGraphEdges(gt, (index+substep)/substep, satdata, maxFSODistance)
		if index%2 == 0 { // QUESTION: why only even index values
			newPath = false

			var err error
			// create edge if two satellites are within maxFSODistance (edge cost calculated from distance)
			graph.SetupGraphSatelliteEdges(gn, index, satdata, maxFSODistance)

			var earthTime time.Time = startTime
			earthTime = earthTime.Add(time.Duration(index * int(timeStep)))
			log.Info().Time("earthTime", earthTime).Int("index", index).Msg("earthTime")
			// log.Debug().Time("time", earthTime).Interface("gs1", groundstations[0].Title).Interface("gs2", groundstations[0].Title).Msg("adding gs edges at earth time")

			// create edge if a GS and satellite are within maxFSODistance (edge cost calculated from distance)
			graph.SetupGraphGroundStationEdges(gn, index, earthTime, satdata, GroundStations, maxFSODistance)

			//Checking path vs new time step
			//Getting the new path
			if len(path) > 0 {
				// shortest path computed from non-negative edges
				// by adding the GS index (from file) to the number of satellites we get the GS vertex index in the graph
				// the path is a slice of integers representing the indexes of the graph's vertices
				nextPath, nextPathDistance, err = graph.GetShortestPath(gn, graphSize, connections[0].Source+len(satdata), connections[0].Destination+len(satdata))
				if len(nextPath) != 0 {
					// if the newly created path and old path are not equivalent, replace old path with new path
					if !slices.Equal(path, nextPath) {
						log.Info().Int64("pathDistance", pathDistance).Int64("nextPathDistance", nextPathDistance).Msg("path change")
						path = nextPath
						pathDistance = nextPathDistance
						newPath = true
					}
				}
				// if there is no path, create a path
			} else {
				path, pathDistance, err = graph.GetShortestPath(gn, graphSize, connections[0].Source+len(satdata), connections[0].Destination+len(satdata))
				if len(path) != 0 {
					newPath = true
					log.Debug().Int64("path_distance", pathDistance).Msg("new path")
				}
			}

			// TODO handle no path available

			if newPath { // QUESTION: will this only be false if graph.GetShortestPath returns a path with 0 elements? and why are we checking for that case?
				// only set a satellite to be active if it is a part of the path
				for _, sat := range satdata {
					sat.Isactive = false
				}
				for _, satellite := range path {
					if satellite < len(satdata) {
						satdata[satellite].Isactive = true
						// TODO modify so groundstations work with tc aswell
					}
				}
				if err != nil {
					log.Error().Err(err).Msg("Error in shortest path")
				}
				log.Info().Ints("path", path).Int("index", index).Ints("containers", SatelliteIdsFromGraphIDs(path...)).Msg("new Path")

				// Setting up the network/route and adding ips to routing
				// prevlinks = activelinks
				nextlinks = []string{}
				for i := 0; i < len(path)-1; i++ {
					linkName := linkNameFromNodeId(path[i], path[i+1])
					nextlinks = append(nextlinks, linkName)
					log.Info().Str("Link", linkName).Strs("nextlinks", nextlinks).Msg("marking link for nextpath")
					// Setting up the network for link

					// iplookup[path[i]] = linkDeatils.NodeOneIP
					// iplookup[path[i+1]] = linkDeatils.NodeTwoIP
				}
				// activelinks subtracted the links which also appear in nextlinks = links that need to be torn down
				linkStopList := linkset.Sub(activelinks, nextlinks)
				// nextlinks subtracted the links which also appear in activelinks = links that need to be setup
				linkStartList := linkset.Sub(nextlinks, activelinks)

				//Newlinks TODO subtract first
				wg := sync.WaitGroup{}
				for _, link := range linkStartList {
					wg.Add(1)
					linkDetails := links[link]
					log.Debug().Interface("link", linkDetails).Msg("Setting up link")
					go func() {
						defer wg.Done()
						podman.SetupLink(linkDetails)
					}()
				}
				// waiting until links have been setup for all links in linkStartList
				wg.Wait()

				// Apply netem to new links
				//* TC command update *//
				simulationTime := index
				wg = sync.WaitGroup{}
				for pathindex := 0; pathindex < len(path)-1; pathindex++ {
					graphid_1 := path[pathindex]
					graphid_2 := path[pathindex+1]
					if graphid_1 >= len(satdata) || graphid_2 >= len(satdata) { // Skip ground stations
						continue
					}
					satFrom := satdata[graphid_1]
					satTo := satdata[graphid_2]
					// if two satellites are within eachothers reach (maxAPDistance)
					if space.Reachable(satFrom.Position[simulationTime], satTo.Position[simulationTime], maxFSODistance) {
						wg.Add(1)
						go func(simulationTime int, satFrom, satTo space.OrbitalData) {
							defer wg.Done()
							distance := satFrom.Position[simulationTime].Distance(satTo.Position[simulationTime])
							latency_ms := space.Latency(distance) * 1000 // QUESTION: why milliseconds here when microsec the other place?
							// Performs a nearly atomic remove/add on an existing node id. If the node does not exist yet it is created.
							cost := int(math.Ceil(latency_ms))
							// TODO: understand tc, qdisc, netem
							command_forward := qdiscCommand("Sat", satTo.SatelliteId, cost)
							container_name_forward := fmt.Sprintf("Sat%d", satFrom.SatelliteId)
							podman.RunCommand(container_name_forward, command_forward)
							command_reverse := qdiscCommand("Sat", satFrom.SatelliteId, cost)
							container_name_reverse := fmt.Sprintf("Sat%d", satTo.SatelliteId)
							podman.RunCommand(container_name_reverse, command_reverse)
						}(simulationTime, satFrom, satTo)
					}
				}
				var gs_name string
				var gs_satellite space.OrbitalData

				gs_name = GroundStations[path[1]-len(SatelliteIds)].Title
				gs_satellite = satdata[path[2]]
				// QUESTION: path[1] gives us the vertices in the path (hashed values?) but how can this give us the index of the GS?
				podman.RunCommand("GS"+GroundStations[path[1]-len(SatelliteIds)].Title, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
				podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))

				gs_name = GroundStations[path[len(path)-(2-1)]-len(SatelliteIds)].Title
				gs_satellite = satdata[path[3-1]]
				podman.RunCommand("GS"+GroundStations[path[len(path)-(2-1)]-len(SatelliteIds)].Title, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
				podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))
				wg.Wait()

				// Setting up the routing table for all containers
				log.Info().Interface("path", path).Msg("debug path")
				// links from setupLinkMap
				routing.LINKS = links

				// slices of commands : "ip route replace destinationIP via nexthopIP"
				commands, reversecommands := routing.RouteTables(path)

				log.Debug().Interface("forward_commands", commands).Msg("FORWARD Routing")
				for container_id, command := range commands {
					if container_id < len(SatelliteIds) {
						podman.RunCommand("Sat"+strconv.Itoa(SatelliteIds[container_id]), command)
					} else {
						podman.RunCommand("GS"+GroundStations[container_id-len(SatelliteIds)].Title, command)
					}
				}
				log.Debug().Interface("reverse_commands", reversecommands).Msg("REVERSE Routing")
				for container_id, command := range reversecommands {
					if container_id < len(SatelliteIds) {
						podman.RunCommand("Sat"+strconv.Itoa(SatelliteIds[container_id]), command)
					} else {
						podman.RunCommand("GS"+GroundStations[container_id-len(SatelliteIds)].Title, command)
					}
				}

				for _, link := range linkStopList {
					linkDetails := links[link]
					log.Debug().Interface("link", linkDetails).Msg("Tearing down link")
					go podman.TearDownLink(linkDetails)
				}
				activelinks = nextlinks

			} else {
				log.Warn().Msg("no path found available")
			}

		}

		//* TC command update *//
		simulationTime := index
		wg := sync.WaitGroup{}
		for pathindex := 0; pathindex < len(path)-1; pathindex++ {
			graphid_1 := path[pathindex]
			graphid_2 := path[pathindex+1]
			if graphid_1 >= len(satdata) || graphid_2 >= len(satdata) {
				continue
			}
			satFrom := satdata[graphid_1]
			satTo := satdata[graphid_2]
			if space.Reachable(satFrom.Position[simulationTime], satTo.Position[simulationTime], maxFSODistance) {
				wg.Add(1)
				go func(simulationTime int, satFrom, satTo space.OrbitalData) {
					defer wg.Done()
					distance := satFrom.Position[simulationTime].Distance(satTo.Position[simulationTime])
					latency_ms := space.Latency(distance) * 1000
					// Performs a nearly atomic remove/add on an existing node id. If the node does not exist yet it is created.
					cost := int(math.Ceil(latency_ms))
					command_forward := qdiscCommand("Sat", satTo.SatelliteId, cost)
					container_name_forward := fmt.Sprintf("Sat%d", satFrom.SatelliteId)
					podman.RunCommand(container_name_forward, command_forward)
					command_reverse := qdiscCommand("Sat", satFrom.SatelliteId, cost)
					container_name_reverse := fmt.Sprintf("Sat%d", satTo.SatelliteId)
					podman.RunCommand(container_name_reverse, command_reverse)
				}(simulationTime, satFrom, satTo)
			}
		}
		var gs_name string
		var gs_satellite space.OrbitalData

		gs_name = GroundStations[path[1]-len(SatelliteIds)].Title
		gs_satellite = satdata[path[2]]
		podman.RunCommand("GS"+GroundStations[path[1]-len(SatelliteIds)].Title, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
		podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))

		gs_name = GroundStations[path[len(path)-(2-1)]-len(SatelliteIds)].Title
		gs_satellite = satdata[path[3-1]]
		podman.RunCommand("GS"+GroundStations[path[len(path)-(2-1)]-len(SatelliteIds)].Title, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
		podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))
		wg.Wait()

		//Wait until next iteration based on time.
		simulationstartCopy := simulationStart
		targetTime := simulationstartCopy.Add(time.Duration(index * int(timeStep)))
		tooSlow := time.Now().After(targetTime)
		if tooSlow {
			log.Warn().Bool("computerIsPotato", tooSlow).Time("targetTime", targetTime).Dur("duration", time.Since(targetTime)).Msg("simulation not running in real time")
		}
		time.Sleep(time.Until((targetTime)))
	}
}

// Installs or replaces a qdisc atomically with the interface equal to satellite id and delay in milliseconds
func qdiscCommand(net_if string, satelliteId int, delay int) string { // QUESTION: what does limit do?
	return fmt.Sprintf("tc qdisc replace dev %s%d root netem delay %dms rate 100mbit limit 500", net_if, satelliteId, delay)
}

func qdiscCommandGS(net_if string, gs_title string) string {
	return fmt.Sprintf("tc qdisc replace dev %s%s root netem delay %dms rate 100mbit limit 500", net_if, gs_title, 0)
}

type connection struct {
	Source      int `parquet:"source"`
	Destination int `parquet:"destination"`
}

func AllConnections(gsdata *[]space.GroundStation) (connections []connection) {
	connectionData := "ElAlamo,Koto\n"
	groundStationPairs := strings.Split(connectionData, "\n")
	// for each connection
	for _, gspair := range groundStationPairs {
		if len(strings.TrimSpace(gspair)) < 2 { // QUESTION: what does this do? will it every be false?
			continue // QUESTION: don't this prevent the rest of the code from running?
		}
		gspairlist := strings.Split(gspair, ",")
		if len(gspairlist) != 2 {
			panic("error in connection data")
		}
		var node1, node2 int = -1, -1
		// for each GroundStation struct in slice
		for i, gs := range *gsdata {
			// if the first ground station specified in 'connectionData' matches the title of one of the GroundStation structs' title
			if gspairlist[0] == gs.Title {
				// assign the index of the GroundStation struct in the slice
				node1 = i
			}
		}
		for i, gs := range *gsdata {
			// if the second ground station specified in 'connectionData' matches the title of one of the GroundStation structs' title
			if gspairlist[1] == gs.Title {
				// assign the index of the GroundStation struct in the slice
				node2 = i
			}
		}
		if node1 == -1 || node2 == -1 {
			panic("could not find groundstation from connection data")
		}
		connections = append(connections, connection{node1, node2})
	}
	return connections
}

// Performs a nearly atomic remove/add on an existing node id. If the node does not exist yet it is created.
func minMax(slice []int) (imin, imax int) {
	imin, imax = -1, -1
	var cmin, cmax int = -1, -1
	if len(slice) != 0 {
		imin, imax = 0, 0
		cmin, cmax = slice[0], slice[0]
	}
	for i, v := range slice {
		if v < cmin {
			imin = i
			cmin = v
		}
		if v > cmax {
			imax = i
			cmax = v
		}
	}
	return imin, imax
}

func setupLinkMap(containers []string, satdata []space.OrbitalData, gsdata []space.GroundStation, connections []connection) map[string]podman.LinkDetails {
	// TODO use connections data for assigning ue data
	octet1 := 120
	octet2 := 130
	octet3 := 0
	octet4 := 0
	cidr := "/29"
	links := make(map[string]podman.LinkDetails)
	// create links between all satellites
	for node1, sat1 := range satdata {
		for node2, sat2 := range satdata {
			if node1 <= node2 { // Ignore half triangle and diagonal
				continue
			}
			// linkname := [node1, node2]
			subnet := strconv.Itoa(octet1) + "." + strconv.Itoa(octet2) + "." + strconv.Itoa(octet3) + "." + strconv.Itoa(octet4) + cidr
			nodeOneIp := strconv.Itoa(octet1) + "." + strconv.Itoa(octet2) + "." + strconv.Itoa(octet3) + "." + strconv.Itoa(octet4+2)
			nodeTwoIp := strconv.Itoa(octet1) + "." + strconv.Itoa(octet2) + "." + strconv.Itoa(octet3) + "." + strconv.Itoa(octet4+3)
			linkDetails := podman.LinkDetails{
				NetworkName: "P7-Link-S" + strconv.Itoa(sat1.SatelliteId) + "-S" + strconv.Itoa(sat2.SatelliteId),
				Subnet:      subnet,
				NodeOneIP:   nodeOneIp,
				NodeOneId:   containers[node1],
				NodeTwoIP:   nodeTwoIp,
				NodeTwoId:   containers[node2],
			}

			// insert satellite link details into map with its key created by linkNameFromNodeId()
			links[linkNameFromNodeId(node1, node2)] = linkDetails
			// log.Debug().Str("name", "S"+strconv.Itoa(node1)+"-S"+strconv.Itoa(node2)).Str("Subnet", links["S"+strconv.Itoa(node1)+"-S"+strconv.Itoa(node2)].Subnet).Msg("Link")
			// change octets to avoid identical ip's
			octet4 += 8
			if octet4 == 248 {
				octet4 = 0
				octet3 += 1
			}
			if octet3 == 255 {
				octet3 = 0
				octet2 += 1
			}
			if octet2 == 255 {
				octet2 = 0
				octet1 += 1
			}
		}
	}
	for node1, gs := range gsdata {
		if !gs.IsAP {
			continue // only make satellite links with access points on the ground
		}
		for node2, sat := range satdata {
			// linkname := [node1, node2]
			subnet := strconv.Itoa(octet1) + "." + strconv.Itoa(octet2) + "." + strconv.Itoa(octet3) + "." + strconv.Itoa(octet4) + cidr
			nodeOneIp := strconv.Itoa(octet1) + "." + strconv.Itoa(octet2) + "." + strconv.Itoa(octet3) + "." + strconv.Itoa(octet4+2)
			nodeTwoIp := strconv.Itoa(octet1) + "." + strconv.Itoa(octet2) + "." + strconv.Itoa(octet3) + "." + strconv.Itoa(octet4+3)
			linkDetails := podman.LinkDetails{
				NetworkName: "P7-Link-G" + gs.Title + "-S" + strconv.Itoa(sat.SatelliteId),
				Subnet:      subnet,
				NodeOneIP:   nodeOneIp,
				NodeOneId:   containers[len(satdata)+node1],
				NodeTwoIP:   nodeTwoIp,
				NodeTwoId:   containers[node2],
			}

			links[linkNameFromNodeId(len(satdata)+node1, node2)] = linkDetails
			// log.Debug().Str("name", "S"+strconv.Itoa(node1)+"-S"+strconv.Itoa(node2)).Str("Subnet", links["S"+strconv.Itoa(node1)+"-S"+strconv.Itoa(node2)].Subnet).Msg("Link")
			octet4 += 8
			if octet4 == 248 {
				octet4 = 0
				octet3 += 1
			}
			if octet3 == 255 {
				octet3 = 0
				octet2 += 1
			}
			if octet2 == 255 {
				octet2 = 0
				octet1 += 1
			}
		}
	}

	for node1, gs1 := range gsdata { // QUESTION: What is this direct link between two gs?
		if !gs1.IsAP {
			continue // only make satellite links with access points on the ground
		}
		for node2, gs2 := range gsdata {
			if gs2.IsAP {
				continue // Disable this for hybrid routing between ground and satellites
			}
			if node1 == node2 {
				continue
			}

			subnet := strconv.Itoa(octet1) + "." + strconv.Itoa(octet2) + "." + strconv.Itoa(octet3) + "." + strconv.Itoa(octet4) + cidr
			nodeOneIp := strconv.Itoa(octet1) + "." + strconv.Itoa(octet2) + "." + strconv.Itoa(octet3) + "." + strconv.Itoa(octet4+2)
			nodeTwoIp := strconv.Itoa(octet1) + "." + strconv.Itoa(octet2) + "." + strconv.Itoa(octet3) + "." + strconv.Itoa(octet4+3)
			linkDetails := podman.LinkDetails{
				NetworkName: "P7-Link-AP" + gs1.Title + "-UE" + gs2.Title,
				Subnet:      subnet,
				NodeOneIP:   nodeOneIp,
				NodeOneId:   containers[len(satdata)+node2], //this was 1 <= QUESTION: What does this refer to?
				NodeTwoIP:   nodeTwoIp,
				NodeTwoId:   containers[len(satdata)+node1],
			}

			links[linkNameFromNodeId(len(satdata)+node1, len(satdata)+node2)] = linkDetails
			// log.Debug().Str("name", "S"+strconv.Itoa(node1)+"-S"+strconv.Itoa(node2)).Str("Subnet", links["S"+strconv.Itoa(node1)+"-S"+strconv.Itoa(node2)].Subnet).Msg("Link")
			octet4 += 8
			if octet4 == 248 {
				octet4 = 0
				octet3 += 1
			}
			if octet3 == 255 {
				octet3 = 0
				octet2 += 1
			}
			if octet2 == 255 {
				octet2 = 0
				octet1 += 1
			}
		}
	}

	return links
}

func SatelliteIdsFromGraphIDs(graphid ...int) (sids []int) {
	for _, gid := range graphid {
		if gid < len(SatelliteIds) {
			sids = append(sids, SatelliteIds[gid])
		} else {
			sids = append(sids, GroundStations[gid-len(SatelliteIds)].ID)
		}
	}
	return sids
}

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

func linkNameFromNodeId(node1, node2 int) string {
	if node1 == node2 {
		panic("aaaaaaaaaaaaa!")
	}
	firstNode := min(node1, node2)
	secondNode := max(node1, node2)
	return "S" + strconv.Itoa(firstNode) + "-S" + strconv.Itoa(secondNode)
}
