from InterPlaneInterSatelliteConnectivity import Constellation
from InterPlaneInterSatelliteConnectivity import GroundSegment
import numpy as np
import pandas as pd

delta_t = 15
N, h, inclination, is_walker_star = Constellation.commercial_constellation_parameters(
    "OneWeb"
)

constellation = Constellation.Constellation(N, h, inclination, is_walker_star)
# constellation.rotate(delta_t)  # coordinates are populated by first rotation
print(constellation)

M = 1000  # time steps
NM = N * M                                                                      # <= QUESTION: Why do we need a length N*M array when we only use N*N?

X, Y, Z = np.empty(NM), np.empty(NM), np.empty(NM)
TimeIndex = np.empty(NM, dtype=np.int64)
SatelliteID = np.empty(NM, dtype=np.int64)
# line_index = 0

# N is the number of satellites
for i in range(M):                                                              # <= QUESTION: Should this be the time M instead?
    constellation.rotate(delta_t)  # coordinates are populated by first rotation
    # for each satellite in constellation, retrieve satellite object
    for i_s, satellite in enumerate(constellation.satellites):
        SatelliteID[(i * N) + i_s] = satellite.id
        TimeIndex[(i * N) + i_s] = i
        X[(i * N) + i_s] = satellite.x
        Y[(i * N) + i_s] = satellite.y
        Z[(i * N) + i_s] = satellite.z
    # line_index+=1
dataframe = pd.DataFrame(
    {
        "satellite_id": SatelliteID,
        "time_index": TimeIndex,
        "pos_x": X,
        "pos_y": Y,
        "pos_z": Z,
    },
)

# write dataframe to parquet file format
dataframe.to_parquet("emulator/constellation.parquet", index=False)
load = pd.read_parquet("emulator/constellation.parquet")
