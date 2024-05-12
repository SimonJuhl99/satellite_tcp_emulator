package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"project/database"
	"project/graph"
	"project/linkset"
	"project/podman"
	"project/routing"
	"project/space"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"
)

const timeStepInt int = 10 // L2 timestep
const timeStepL3 int = 20
const testTCPversion string = "cubic"
const printOn bool = false
const noDrop bool = false
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

	startIperfTime := 90
	startTCPmetricsTime := 120
	startIperfIndex := startIperfTime / timeStepInt
	startTCPmetricsIndex := startTCPmetricsTime / timeStepInt

	log.Info().Int("timeStepInt", timeStepInt).Msg("====================>")
	log.Info().Int("startIperfIndex", startIperfIndex).Msg("====================>")
	log.Info().Int("startTCPmetricsIndex", startTCPmetricsIndex).Msg("====================>")

	// create slice of OrbitalData structs
	var satdata []space.OrbitalData
	var satdata1sec []space.OrbitalData
	startTime := time.Date(2022, 9, 11, 12, 00, 00, 00, time.UTC)
	endTime := time.Date(2022, 9, 21, 22, 00, 00, 00, time.UTC)
	timeStep := time.Duration(timeStepInt) * time.Second
	oneSecond := time.Duration(1) * time.Second
	duration := endTime.Sub(startTime)
	// retrieve data generated in satellite_positions.py (based on Israels simulation)
	var satellite_positions_path string = "./constellation-delta" + strconv.Itoa(timeStepInt) + ".parquet"

	// =========== Load satellite positions ===========
	log.Info().Msg("using simulated constellation")
	// returns slice of OrbitalData structs, each struct containing positions (LatLong in degrees) for one satellite over time
	satdata = database.LoadSatellitePositions(satellite_positions_path, constellation_name, startTime, timeStep, 1000)
	satdata1sec = database.LoadSatellitePositions("./constellation-oneweb-delta1.parquet", constellation_name, startTime, oneSecond, 2000)
	graphSize += len(satdata)
	for _, orbitialData := range satdata {
		SatelliteIds = append(SatelliteIds, orbitialData.SatelliteId)
	}
	// ==============================================

	log.Info().Int("satelliteCount", len(SatelliteIds)).Msg("Found satellites")

	sort.Ints(SatelliteIds) //satdata is sorted in GetSatData. SatelliteIds must be sorted to be used as common indexing

	log.Debug().Msg("Satdata vs satdata1sec")
	for idx, da := range satdata[5].Position {
		if idx > 3 {
			break
		} else {
			log.Info().Interface("satdata", da).Int("idx", idx).Msg("?????")
		}
	}
	for idx, da := range satdata1sec[5].Position {
		if idx > 21 {
			break
		} else {
			log.Info().Interface("satdata", da).Int("idx", idx).Msg("?????")
		}
	}

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

	var path, nextPath, prevPath []int
	prevSatsL2Path := make([]int, 0)
	var pathDistance, nextPathDistance int64
	var newPath bool = false
	simulationStart := time.Now()
	log.Info().Time("simulationStart", simulationStart).Msg("starting simulation")

	f, err := os.Create("/tmp/route-changes-update-L3-every-" + strconv.Itoa(timeStepL3) + "-seconds") //+ strings.ToLower(constellation_name))
	if err != nil {
		log.Error().Err(err).Msg("Error in creating route change file")
	}
	defer f.Close()

	for index := 0; index < (int(duration)/(int(timeStep)))-1; index++ {

		select {
		case stopsignal := <-interruptSignal:
			log.Info().Interface("signal", stopsignal).Msg("shutting down")
			time.Sleep(2 * time.Second)
			return
		default:
		}

		if (index*timeStepInt)%timeStepL3 == 0 { // if L2 timestep is a multiple of L3 timestep
			log.Info().Msg("\n\n======================================\nL3 UPDATE\n======================================\n")

			newPath = false
			prevSats := make([]int, 0)
			prevSatsL2Path = make([]int, 0)

			var err error
			// create edge if two satellites are within maxFSODistance (edge cost calculated from distance)
			graph.SetupGraphSatelliteEdges(gn, index, satdata, maxFSODistance)

			var earthTime time.Time = startTime
			earthTime = earthTime.Add(time.Duration(index * int(timeStep)))
			log.Info().Time("earthTime", earthTime).Int("index", index).Msg("earthTime")
			log.Info().Msg("Time right now: " + strconv.Itoa((index-startTCPmetricsIndex)*int(timeStep)/1000000000))
			// log.Debug().Time("time", earthTime).Interface("gs1", groundstations[0].Title).Interface("gs2", groundstations[0].Title).Msg("adding gs edges at earth time")

			// create edge if a GS and satellite are within maxFSODistance (edge cost calculated from distance)
			graph.SetupGraphGroundStationEdges(gn, index, satdata, GroundStations, maxFSODistance)

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
						var pathUnits string = ""
						for unit := 0; unit < len(nextPath); unit++ {
							pathUnits += containers[nextPath[unit]] + " "
						}
						var pathInfo string = "Path change found at time " + strconv.Itoa((index-startTCPmetricsIndex)*int(timeStep)/1000000000) + "\t" + pathUnits + "\n"
						log.Info().Msg("Time of path change: " + strconv.Itoa((index-startTCPmetricsIndex)*int(timeStep)/1000000000))
						_, err := f.WriteString(pathInfo)
						if err != nil {
							log.Error().Err(err).Msg("Error writing new path to file")
						}
						f.Sync()

						//log.Info().Int64("pathDistance", pathDistance).Int64("nextPathDistance", nextPathDistance).Msg("path change")
						if noDrop {
							prevPath = path
						}

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

			if newPath {
				// extract all IDs of satellites that are not part of new path
				if noDrop {
					for i := 0; i < len(prevPath); i++ {
						found := false
						for j := 0; j < len(path); j++ {
							if path[j] == prevPath[i] {
								found = true
							}
						}
						if !found {
							prevSats = append(prevSats, prevPath[i])
						}
					}
					//log.Info().Ints("prevPath", prevPath).Int("time index", index).Msg("Previous path")
					//log.Info().Ints("prevSats", prevSats).Int("time index", index).Msg("Sats that are not part of new path")
				}

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
				// activate satellites from previous path
				if noDrop {
					for _, satellite := range prevSats {
						if satellite < len(satdata) {
							satdata[satellite].Isactive = true
						}
					}
				}
				if err != nil {
					log.Error().Err(err).Msg("Error in shortest path")
				}
				//log.Info().Ints("path", path).Int("index", index).Ints("containers", SatelliteIdsFromGraphIDs(path...)).Msg("new Path")
				log.Info().Ints("path", path).Int("time index", index).Msg("New path")

				// Setting up the network/route and adding ips to routing
				// prevlinks = activelinks
				nextlinks = []string{}
				for i := 0; i < len(path)-1; i++ {
					linkName := linkNameFromNodeId(path[i], path[i+1])
					nextlinks = append(nextlinks, linkName)
					//log.Info().Str("Link", linkName).Strs("nextlinks", nextlinks).Msg("marking link for nextpath")
					// Setting up the network for link

					// iplookup[path[i]] = linkDeatils.NodeOneIP
					// iplookup[path[i+1]] = linkDeatils.NodeTwoIP
				}
				//log.Info().Strs("nextlinks before prevpath", nextlinks).Msg("link for nextpath")

				// find links between prevSats and their previous neighbors, append the link names to nextlinks to avoid that these links are torn down
				if noDrop {
					lastJ := -1
					for i := 0; i < len(prevSats); i++ {
						for j := 0; j < len(prevPath); j++ {
							if prevSats[i] == prevPath[j] {
								idxNeighbor1 := j + 1
								linkName := linkNameFromNodeId(prevPath[j], prevPath[idxNeighbor1])
								nextlinks = append(nextlinks, linkName)
								// if the neighbor with a lower index value has in the previous iteration already had links created to this satellite
								if lastJ != (j - 1) {
									idxNeighbor2 := j - 1
									linkName = linkNameFromNodeId(prevPath[j], prevPath[idxNeighbor2])
									nextlinks = append(nextlinks, linkName)
								}
								lastJ = j
							}
						}
					}
					//log.Info().Strs("nextlinks after prevpath", nextlinks).Msg("link for nextpath")
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
					if printOn {
						log.Debug().Interface("link", linkDetails).Msg("Setting up link")
					}
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
							// each satellite will have two interfaces (one for each neighboring satellite)
							// the interfaces on the satFrom satellite will have the IDs of the satTo satellites (this is used to indicate which channel is being emulated)
							command_forward := qdiscCommand("Sat", satTo.SatelliteId, cost)
							container_name_forward := fmt.Sprintf("Sat%d", satFrom.SatelliteId)
							podman.RunCommand(container_name_forward, command_forward)
							command_reverse := qdiscCommand("Sat", satFrom.SatelliteId, cost)
							container_name_reverse := fmt.Sprintf("Sat%d", satTo.SatelliteId)
							podman.RunCommand(container_name_reverse, command_reverse)
						}(simulationTime, satFrom, satTo)
					}
				}

				// make path which enable satellties in prevSats to get remaining packets onto the main path
				if noDrop {
					lastJ := -1
					//log.Info().Interface("prevSats", prevSats).Msg("making prevSatsL2Path")
					//log.Info().Interface("prevPath", prevPath).Msg("making prevSatsL2Path")
					for i := 0; i < len(prevSats); i++ {
						for j := 0; j < len(prevPath); j++ {
							// j will always be equal to 2 or larger
							if prevSats[i] == prevPath[j] {
								// only add the current satellite if it is MORE than 1 iteration since a satellite was added (or if it is the first time adding one)
								if lastJ != (j - 1) {
									// only add the "before-neighbor" if it is MORE than 2 iterations since a satellite was added (or if it is the first time adding one)
									if lastJ != (j - 2) {
										idxNeighbor1 := j - 1
										prevSatsL2Path = append(prevSatsL2Path, prevPath[idxNeighbor1])
									}
									prevSatsL2Path = append(prevSatsL2Path, prevPath[j])
								}
								// always add the "after-neighbor"
								idxNeighbor2 := j + 1
								prevSatsL2Path = append(prevSatsL2Path, prevPath[idxNeighbor2])

								lastJ = j
							}
						}
					}
					log.Info().Msg("\n========= Previous sats L2 path =========")
					log.Info().Interface("prevSatsL2Path", prevSatsL2Path).Msg("cmon")
				}

				// Apply netem to prevSats links
				//* TC command update *//
				if noDrop {
					for pathindex := 0; pathindex < len(prevSatsL2Path)-1; pathindex++ {
						graphid_1 := prevSatsL2Path[pathindex]
						graphid_2 := prevSatsL2Path[pathindex+1]
						if graphid_1 >= len(satdata) && graphid_2 >= len(satdata) {
							continue
						} else if graphid_1 >= len(satdata) { // if first node is a gs
							gs_name := GroundStations[graphid_1-len(satdata)].Title
							gs_satellite := satdata[graphid_2]
							wg.Add(1)
							go func(simulationTime int, gs_name string, satTo space.OrbitalData) {
								defer wg.Done()
								podman.RunCommand("GS"+gs_name, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
								podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))
							}(simulationTime, gs_name, gs_satellite)
						} else if graphid_2 >= len(satdata) { // if last node is a gs
							gs_name := GroundStations[graphid_2-len(satdata)].Title
							gs_satellite := satdata[graphid_1]
							wg.Add(1)
							go func(simulationTime int, gs_name string, satTo space.OrbitalData) {
								defer wg.Done()
								podman.RunCommand("GS"+gs_name, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
								podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))
							}(simulationTime, gs_name, gs_satellite)
						} else { // if both nodes are sats
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
									podman.RunCommand(fmt.Sprintf("Sat%d", satFrom.SatelliteId), qdiscCommand("Sat", satTo.SatelliteId, cost))
									podman.RunCommand(fmt.Sprintf("Sat%d", satTo.SatelliteId), qdiscCommand("Sat", satFrom.SatelliteId, cost))
								}(simulationTime, satFrom, satTo)
							}
						}
					}

					log.Info().Msg("\n")
				}

				var gs_name string
				var gs_satellite space.OrbitalData

				// TODO make function that can make gs-sat link by passing GS_ID, SAT_ID, GroundStations and satdata (and at some point COST as well)

				gs_name = GroundStations[path[1]-len(SatelliteIds)].Title
				gs_satellite = satdata[path[2]]
				podman.RunCommand("GS"+gs_name, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
				podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))

				gs_name = GroundStations[path[len(path)-2]-len(SatelliteIds)].Title
				gs_satellite = satdata[path[len(path)-3]]
				//podman.RunCommand("GS"+GroundStations[path[len(path)-(2-1)]-len(SatelliteIds)].Title, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
				podman.RunCommand("GS"+gs_name, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
				podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))
				wg.Wait()

				// Setting up the routing table for all containers
				if printOn {
					log.Info().Interface("path", path).Msg("debug path")
				}
				// links from setupLinkMap
				routing.LINKS = links

				// slices of commands : "ip route replace destinationIP via nexthopIP"
				commands, reversecommands := routing.RouteTables(path)

				// output commands that will only allow packets to be routed AWAY from the old sats
				commandsPrevsats, reversecommandsPrevsats := routing.RouteTablesPrevSats(path, prevSats, prevSatsL2Path)

				wg.Wait()

				// Apply netem to new links
				//* TC command update *//

				wg = sync.WaitGroup{}

				if printOn {
					log.Debug().Interface("forward_commands", commands).Msg("FORWARD Routing")
				}
				for container_id, command := range commands {
					wg.Add(1)
					if container_id < len(SatelliteIds) {
						go func(container_id int, command string) {
							defer wg.Done()
							podman.RunCommand("Sat"+strconv.Itoa(SatelliteIds[container_id]), command)
						}(container_id, command)
					} else {
						go func(container_id int, command string) {
							defer wg.Done()
							podman.RunCommand("GS"+GroundStations[container_id-len(SatelliteIds)].Title, command)
						}(container_id, command)
					}
				}
				if printOn {
					log.Debug().Interface("reverse_commands", reversecommands).Msg("REVERSE Routing")
				}
				for container_id, command := range reversecommands {
					wg.Add(1)
					if container_id < len(SatelliteIds) {
						go func(container_id int, command string) {
							defer wg.Done()
							podman.RunCommand("Sat"+strconv.Itoa(SatelliteIds[container_id]), command)
						}(container_id, command)
					} else {
						go func(container_id int, command string) {
							defer wg.Done()
							podman.RunCommand("GS"+GroundStations[container_id-len(SatelliteIds)].Title, command)
						}(container_id, command)
					}
				}
				if printOn {
					log.Debug().Interface("forward_commands", commandsPrevsats).Msg("FORWARD Routing")
				}
				for container_id, command := range commandsPrevsats {
					wg.Add(1)
					if container_id < len(SatelliteIds) {
						go func(container_id int, command string) {
							defer wg.Done()
							podman.RunCommand("Sat"+strconv.Itoa(SatelliteIds[container_id]), command)
						}(container_id, command)
					} else {
						go func(container_id int, command string) {
							defer wg.Done()
							podman.RunCommand("GS"+GroundStations[container_id-len(SatelliteIds)].Title, command)
						}(container_id, command)
					}
				}
				if printOn {
					log.Debug().Interface("reverse_commands", reversecommandsPrevsats).Msg("REVERSE Routing")
				}
				for container_id, command := range reversecommandsPrevsats {
					wg.Add(1)
					if container_id < len(SatelliteIds) {
						go func(container_id int, command string) {
							defer wg.Done()
							podman.RunCommand("Sat"+strconv.Itoa(SatelliteIds[container_id]), command)
						}(container_id, command)
					} else {
						go func(container_id int, command string) {
							defer wg.Done()
							podman.RunCommand("GS"+GroundStations[container_id-len(SatelliteIds)].Title, command)
						}(container_id, command)
					}
				}
				wg.Wait()

				for _, link := range linkStopList {
					linkDetails := links[link]
					if printOn {
						log.Debug().Interface("link", linkDetails).Msg("Tearing down link")
					}
					go podman.TearDownLink(linkDetails)
				}

				activelinks = nextlinks

			} else {
				log.Warn().Msg("no path found available")
			}

		}

		log.Info().Msg("\n\n======================================\nL2 UPDATE\n======================================\n")

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

		// Apply netem to prevSats links
		//* TC command update *//
		if noDrop {
			for pathindex := 0; pathindex < len(prevSatsL2Path)-1; pathindex++ {
				graphid_1 := prevSatsL2Path[pathindex]
				graphid_2 := prevSatsL2Path[pathindex+1]
				if graphid_1 >= len(satdata) && graphid_2 >= len(satdata) {
					continue
				} else if graphid_1 >= len(satdata) { // if first node is a gs
					gs_name := GroundStations[graphid_1-len(satdata)].Title
					gs_satellite := satdata[graphid_2]
					podman.RunCommand("GS"+gs_name, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
					podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))
				} else if graphid_2 >= len(satdata) { // if last node is a gs
					gs_name := GroundStations[graphid_2-len(satdata)].Title
					gs_satellite := satdata[graphid_1]
					podman.RunCommand("GS"+gs_name, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
					podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))
				} else { // if both nodes are sats
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
							podman.RunCommand(fmt.Sprintf("Sat%d", satFrom.SatelliteId), qdiscCommand("Sat", satTo.SatelliteId, cost))
							podman.RunCommand(fmt.Sprintf("Sat%d", satTo.SatelliteId), qdiscCommand("Sat", satFrom.SatelliteId, cost))
						}(simulationTime, satFrom, satTo)
					}
				}
			}
		}

		var gs_name string
		var gs_satellite space.OrbitalData

		gs_name = GroundStations[path[1]-len(SatelliteIds)].Title
		gs_satellite = satdata[path[2]]
		podman.RunCommand("GS"+gs_name, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
		podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))

		gs_name = GroundStations[path[len(path)-2]-len(SatelliteIds)].Title
		gs_satellite = satdata[path[len(path)-3]]
		//podman.RunCommand("GS"+GroundStations[path[len(path)-(2-1)]-len(SatelliteIds)].Title, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
		podman.RunCommand("GS"+gs_name, qdiscCommand("Sat", gs_satellite.SatelliteId, 0))
		podman.RunCommand(fmt.Sprintf("Sat%d", gs_satellite.SatelliteId), qdiscCommandGS("GS", gs_name))
		wg.Wait()

		// start measuring tcp performance
		if index == startIperfIndex {
			startTesting1()
			log.Info().Msg("Iperf started")
		} else if index == startTCPmetricsIndex {
			startTesting2()
			log.Info().Msg("TCP_metrics started")
		} else if ((index - startTCPmetricsIndex) * (int(timeStep) / 1000000000)) > 1000 { // divide by 10e9 as timestep is in nanoseconds
			log.Info().Msg("\n\n\n\n\tTEST OVER - 1000 SECONDS PASSED ")
			log.Info().Int("seconds passed", (index-startTCPmetricsIndex)*int(timeStep)/1000000000).Msg("")
			log.Info().Msg("\n\n\n\n")
		}

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

func startTesting1() {
	cmd := exec.Command("/bin/bash", "-c", "sudo podman container inspect GSKoto | grep  IPAddress | tail -n1")
	stdout, err := cmd.Output()

	r, _ := regexp.Compile(`[0-9\.]+`)
	regexstr := (r.FindString(string(stdout)))

	if err != nil {
		log.Error().Msg("bash command failed")
	}
	log.Info().Str("IP", regexstr).Msg("GSKoto")

	podman.RunCommand("GSKoto", "iperf3 -s -p 9191")
	podman.RunCommand("GSElAlamo", "iperf3 -c "+regexstr+" -p 9191 -t 2000 -C "+testTCPversion)
}

func startTesting2() {
	podman.RunCommand("GSKoto", "./tcp_metrics -t 1000")
	podman.RunCommand("GSElAlamo", "./tcp_metrics -t 1000")
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
