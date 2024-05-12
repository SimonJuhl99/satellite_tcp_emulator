package database

import (
	"context"
	"errors"
	"log"
	"project/space"
	"strconv"
	"time"

	qdb "github.com/questdb/go-questdb-client"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/reader"
	"github.com/xitongsys/parquet-go/writer"
)

type SatelliteLineData struct {
	SatelliteID int
	Title       string
	Index       uint
	Position    space.Vector3
	Velocity    space.Vector3
	LatLong     space.LatLong
	Timestamp   int64
}

type FlatSatelliteLineData struct {
	SatelliteID int32   `json:"satellite_id" parquet:"name=satellite_id, type=INT32, convertedtype=INT_32"`
	Index       int32   `json:"time_index" parquet:"name=time_index, type=INT32, convertedtype=INT_32"`
	PosX        float64 `json:"pos_x" parquet:"name=pos_x, type=DOUBLE"`
	PosY        float64 `json:"pos_y" parquet:"name=pos_y, type=DOUBLE"`
	PosZ        float64 `json:"pos_z" parquet:"name=pos_z, type=DOUBLE"`
	VelX        float64 `json:"vel_x" parquet:"name=vel_x, type=DOUBLE"`
	VelY        float64 `json:"vel_y" parquet:"name=vel_y, type=DOUBLE"`
	VelZ        float64 `json:"vel_z" parquet:"name=vel_z, type=DOUBLE"`
	Lattitude   float64 `json:"latitude" parquet:"name=latitude, type=DOUBLE"`
	Longitude   float64 `json:"longitude" parquet:"name=longitude, type=DOUBLE"`
	Timestamp   int64   `parquet:"name=timestamp, type=INT64, convertedtype=TIMESTAMP_MILLIS"`
}

//	type FlatSatelliteLineDataIsrael struct {
//		SatelliteID int64   `json:"satellite_id" parquet:"name=satellite_id, type=INT64, convertedtype=INT_64"`
//		Index       int64   `json:"time_index" parquet:"name=time_index, type=INT64, convertedtype=INT_64"`
//		PosX        float64 `json:"pos_x" parquet:"name=pos_x, type=DOUBLE"`
//		PosY        float64 `json:"pos_y" parquet:"name=pos_y, type=DOUBLE"`
//		PosZ        float64 `json:"pos_z" parquet:"name=pos_z, type=DOUBLE"`
//		// Timestamp   int64   `parquet:"name=timestamp, type=INT64, convertedtype=TIMESTAMP_MILLIS"`
//	}
type FlatSatelliteLineDataIsrael struct {
	SatelliteID *int64   `json:"satellite_id" parquet:"name=satellite_id, type=INT64"`
	Index       *int64   `json:"time_index" parquet:"name=time_index, type=INT64"`
	PosX        *float64 `json:"pos_x" parquet:"name=pos_x, type=DOUBLE"`
	PosY        *float64 `json:"pos_y" parquet:"name=pos_y, type=DOUBLE"`
	PosZ        *float64 `json:"pos_z" parquet:"name=pos_z, type=DOUBLE"`
	// Timestamp   int64   `parquet:"name=timestamp, type=INT64, convertedtype=TIMESTAMP_MILLIS"`
}

type FlatGroundStationLineDataIsrael struct {
	//GroundStationTitle *string  `json:"groundstation_title" parquet:"name=groundstation_title, type=BYTE_ARRAY"`
	GroundStationID *int64   `json:"groundstation_id" parquet:"name=groundstation_id, type=INT64"`
	Index           *int64   `json:"time_index" parquet:"name=time_index, type=INT64"`
	Latitude        *float64 `json:"latitude" parquet:"name=latitude, type=DOUBLE"`
	Longitude       *float64 `json:"longitude" parquet:"name=longitude, type=DOUBLE"`
	PosX            *float64 `json:"pos_x" parquet:"name=pos_x, type=DOUBLE"`
	PosY            *float64 `json:"pos_y" parquet:"name=pos_y, type=DOUBLE"`
	PosZ            *float64 `json:"pos_z" parquet:"name=pos_z, type=DOUBLE"`
	// Timestamp   int64   `parquet:"name=timestamp, type=INT64, convertedtype=TIMESTAMP_MILLIS"`
}

func WriteLogs(fname string, datatype interface{}) (filewriter *writer.ParquetWriter, stop func()) {
	var err error
	fw, err := local.NewLocalFileWriter(fname + time.Now().Format(time.StampMilli) + ".parquet")
	if err != nil {
		return nil, nil
	}

	//write
	pw, err := writer.NewParquetWriter(fw, datatype, 2)
	if err != nil {
		return nil, nil
	}

	pw.RowGroupSize = 1 * 256 * 1024 //256K
	pw.PageSize = 2 * 1024           //2K
	pw.CompressionType = parquet.CompressionCodec_SNAPPY

	Stop := func() {
		pw.Flush(true)
		if err = pw.WriteStop(); err != nil {
			return
		}
		fw.Close()
	}

	return pw, Stop

}

func WriteWorker(lineData <-chan SatelliteLineData, database_address string) error {
	var address string = database_address
	if database_address == "" {
		address = "100.113.13.30:9009" // TODO: Important
	}
	ctx := context.Background()
	// Connect to QuestDB running on 127.0.0.1:9009
	sender, err := qdb.NewLineSender(ctx, qdb.WithAddress(address)) // https://questdb.io/docs/reference/api/ilp/overview/
	if err != nil {
		return errors.New("failed to connect to QuestDB: " + err.Error())
	}
	// Make sure to close the sender on exit to release resources.
	defer sender.Close()
	// Send a few ILP messages.
	for data := range lineData {
		err = sender.
			Table("satellites").
			Symbol("satellite_name", data.Title).
			Int64Column("satellite_id", int64(data.SatelliteID)). // QUESTION: Why is ID not a symbol when name is?
			Int64Column("time_index", int64(data.Index)).
			Float64Column("XPos", data.Position.X).
			Float64Column("YPos", data.Position.Y).
			Float64Column("ZPos", data.Position.Z).
			Float64Column("XVel", data.Velocity.X).
			Float64Column("YVel", data.Velocity.Y).
			Float64Column("ZVel", data.Velocity.Z).
			Float64Column("Latitude", data.LatLong.Latitude).
			Float64Column("Longitude", data.LatLong.Longitude).
			At(context.Background(), data.Timestamp) // finalizes the ILP message
		if err != nil {
			log.Println(err)
		}
	}

	// Make sure that the messages are sent over the network.
	err = sender.Flush(ctx)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

// Takes as input: the satellite positions over time (retrieved from "satellite_positions.py")
// the positions (LatLong in radians) are used to calculate the LatLong in degrees.
// Outputs: All satellites with their information are stored as OrbitalData instances in a slice
func LoadSatellitePositions(fileName string, constellation string, startTime time.Time, timestep time.Duration, satelliteTimeSteps int) []space.OrbitalData {

	log.Printf("opening file\n")
	fr, err := local.NewLocalFileReader(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer fr.Close()

	log.Printf("opening reader")
	pr, err := reader.NewParquetReader(fr, new(FlatSatelliteLineDataIsrael), 4)
	if err != nil {
		log.Fatal(err)
	}
	defer pr.ReadStop()
	log.Println(pr.Footer.Schema)

	log.Printf("reading row count")
	row_count := pr.GetNumRows()
	log.Printf("%d\n", row_count)

	NumberOfSatellites := 0

	if constellation == "Kepler" {
		NumberOfSatellites = 140
	} else if constellation == "OneWeb" {
		NumberOfSatellites = 648
	} else if constellation == "Starlink" {
		NumberOfSatellites = 1584
	} else {
		log.Fatal("constellation was not correctly specified", constellation)
	}

	expected_row_count := NumberOfSatellites * satelliteTimeSteps
	if int(row_count) != expected_row_count { // TODO: Make a check that not only compares length but checks if entries are empty
		log.Fatal("parquet file length does not match number of expected elements", row_count)
	}

	// make slice of OrbitalData structs
	satdata := make([]space.OrbitalData, NumberOfSatellites)
	// create one OrbitalData instance for each satellite, give it ID and title
	for sid := 0; sid < NumberOfSatellites; sid++ {
		// log.Printf("sid %d \n", sid)

		orbitalData := space.OrbitalData{
			Isactive:    true,
			SatelliteId: sid,
			Title:       strconv.Itoa(sid), // int to string
			Position:    make([]space.Vector3, satelliteTimeSteps),
			Velocity:    make([]space.Vector3, satelliteTimeSteps),
			LatLong:     make([]space.LatLong, satelliteTimeSteps),
		}
		satdata[sid] = orbitalData
	}
	//tstart := time.Date(2023, 5, 9, 12, 0, 0, 0, time.UTC)
	tstart := startTime

	for timeindex := 0; timeindex < satelliteTimeSteps; timeindex++ {

		// make slice of FlatSatelliteLineDataIsrael structs
		lines := make([]FlatSatelliteLineDataIsrael, NumberOfSatellites)
		// use parquet reader to unmarshal the rows of the parquet file to the slice of FlatSatelliteLineDataIsrael-structs
		err = pr.Read(&lines)
		if err != nil {
			log.Fatal(err)
		}
		// log.Printf("copying data for sid %d index: %d\n", sid, index)

		// each line is a FlatSatelliteLineDataIsrael instance | the outer for-loop controls what timeindex we view the satellite at
		for satellite_index, line := range lines {
			// log.Printf("position %d \n%v", index, line)

			position := space.Vector3{X: *line.PosX / 1000, Y: *line.PosY / 1000, Z: *line.PosZ / 1000}
			satdata[satellite_index].Position[timeindex] = position
			// input: earth-centered intertial LatLong coordinates in radians | returns LatLong coordinates in degrees
			satdata[satellite_index].LatLong[timeindex] = space.LLAFromPosition(position, tstart)
		}
		tstart = tstart.Add(timestep)
	}

	return satdata
}

func LoadGroundStationPositions(fileName string, startTime time.Time, timestep time.Duration, groundstationTimeSteps int) []space.GroundStation {

	gs_list := [4]string{"Madrid", "ElAlamo", "Tokyo", "Koto"}

	log.Printf("opening file\n")
	fr, err := local.NewLocalFileReader(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer fr.Close()

	log.Printf("opening reader")
	pr, err := reader.NewParquetReader(fr, new(FlatGroundStationLineDataIsrael), 4)
	if err != nil {
		log.Fatal(err)
	}
	defer pr.ReadStop()
	log.Println(pr.Footer.Schema)

	log.Printf("reading row count")
	row_count := pr.GetNumRows()
	log.Printf("%d\n", row_count)

	NumberOfGroundStations := 4

	expected_row_count := NumberOfGroundStations * groundstationTimeSteps
	if int(row_count) != expected_row_count { // TODO: Make a check that not only compares length but checks if entries are empty
		log.Fatal("parquet file length does not match number of expected elements", row_count)
	}

	// make slice of OrbitalData structs
	gsdata := make([]space.GroundStation, NumberOfGroundStations)
	// create one OrbitalData instance for each satellite, give it ID and title
	for sid := 0; sid < NumberOfGroundStations; sid++ {
		// log.Printf("sid %d \n", sid)

		groundstationData := space.GroundStation{
			Title:    "",
			ID:       sid,
			Latlong:  space.LatLong{Latitude: 0.0, Longitude: 0.0},
			Position: make([]space.Vector3, groundstationTimeSteps),
			IsAP:     true,
			Isactive: false,
		}
		gsdata[sid] = groundstationData
	}
	//tstart := time.Date(2023, 5, 9, 12, 0, 0, 0, time.UTC)
	tstart := startTime

	for timeindex := 0; timeindex < groundstationTimeSteps; timeindex++ {

		// make slice of FlatGroundStationLineDataIsrael structs
		lines := make([]FlatGroundStationLineDataIsrael, NumberOfGroundStations)
		// use parquet reader to unmarshal the rows of the parquet file to the slice of FlatSatelliteLineDataIsrael-structs
		err = pr.Read(&lines)
		if err != nil {
			log.Fatal(err)
		}
		// log.Printf("copying data for sid %d index: %d\n", sid, index)

		// each line is a FlatGroundStationLineDataIsrael instance | the outer for-loop controls what timeindex we view the satellite at
		for groundstation_index, line := range lines {
			gsdata[groundstation_index].Title = gs_list[groundstation_index]
			gsdata[groundstation_index].Latlong = space.LatLong{Latitude: *line.Latitude, Longitude: *line.Longitude}
			position := space.Vector3{X: *line.PosX / 1000, Y: *line.PosY / 1000, Z: *line.PosZ / 1000}
			gsdata[groundstation_index].Position[timeindex] = position
			// input: earth-centered intertial LatLong coordinates in radians | returns LatLong coordinates in degrees
			if gs_list[groundstation_index] == "ElAlamo" || gs_list[groundstation_index] == "Koto" {
				gsdata[groundstation_index].IsAP = false
			}
		}
		tstart = tstart.Add(timestep)
	}

	return gsdata
}
