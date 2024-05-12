from InterPlaneInterSatelliteConnectivity import Constellation
import numpy as np
import pandas as pd
import math

# Madrid, 40.228732, -4.010844, true
# ElAlamo, 40.231081686, -3.9943653458, false
# Tokyo, 35.678892, 139.768596, true
# Koto, 35.65081211, 139.8120700, false

# ,ID,location,latitude_deg,longitude_deg

groundstation_initialization = [
    ["Madrid", 40.228732, -4.010844], 
    ["ElAlamo", 40.231081686, -3.9943653458], 
    ["Tokyo", 35.678892, 139.768596], 
    ["Koto", 35.65081211, 139.8120700]
]

delta_t = 1

groundsegment = Constellation.GroundSegment(groundstation_initialization)
# constellation.rotate(delta_t)  # coordinates are populated by first rotation
N = len(groundsegment.ground_stations)
M = 5000  # time steps
NM = N * M

X, Y, Z = np.empty(NM), np.empty(NM), np.empty(NM)
TimeIndex = np.empty(NM, dtype=np.int64)
Latitude = np.empty(NM, dtype=np.float64)
Longitude = np.empty(NM, dtype=np.float64)
GroundstationID = np.empty(NM, dtype=np.int64)
#GroundstationTitle = np.empty(NM, dtype="S10")
# line_index = 0

# N is the number of satellites
# For each time step rotate the constellation
for i in range(M):
    groundsegment.rotate(delta_t)  # coordinates are populated by first rotation
    # for each satellite in rotated constellation, retrieve satellite object
    for i_s, groundstation in enumerate(groundsegment.ground_stations):
        #GroundstationTitle[(i * N) + i_s] = groundstation.location
        GroundstationID[(i * N) + i_s] = groundstation.id
        Latitude[(i * N) + i_s] = groundstation.latitude_deg
        Longitude[(i * N) + i_s] = groundstation.longitude_deg
        TimeIndex[(i * N) + i_s] = i
        X[(i * N) + i_s] = groundstation.x
        Y[(i * N) + i_s] = groundstation.y
        Z[(i * N) + i_s] = groundstation.z
    # line_index+=1
dataframe = pd.DataFrame(
    {
        #"title": GroundstationTitle,
        "groundstation_id": GroundstationID,
        "time_index": TimeIndex,
        "latitude": Latitude,
        "longitude": Longitude,
        "pos_x": X,
        "pos_y": Y,
        "pos_z": Z,
    },
)

# write dataframe to parquet file format
dataframe.to_parquet("emulator/groundstation-delta1_5k.parquet", index=False)
#load = pd.read_parquet("emulator/constellation-delta1.parquet")
