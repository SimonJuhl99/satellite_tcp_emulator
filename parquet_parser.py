import pandas as pd
from matplotlib import pyplot as plt
import numpy as np
import time

#df = pd.read_parquet('./emulator/satdata.parquet', engine='fastparquet')
#df = pd.read_parquet('/home/simon/Documents/tcp_stats_1.parquet', engine='fastparquet')
df = pd.read_parquet('./tcp_stats_cubic_1000.parquet', engine='fastparquet')
#df = pd.read_parquet('./emulator/tcp_statistics6:27PM.parquet', engine='fastparquet')



x = []
y = []

start_time = df['timestamp'][0].to_pydatetime()
unix_start_time = time.mktime(start_time.timetuple())

for i, el in enumerate(df['timestamp']):
    if int(df['id'][i]) == 0:
        time_string = df['timestamp'][i].to_pydatetime()
        unix_time_stamp = time.mktime(time_string.timetuple()) - unix_start_time
        x.append(int(unix_time_stamp))
        y.append(df['send_rate'][i])
        #print((df['timestamp'][i]), df['rtt_mean'][i])
    #print(df['id'][i], df['timestamp'][i], df['rtt_mean'][i], df['rtt_var'][i])

plt.plot(np.asarray(x, int), y)

plt.show()

print(df)
print(df.columns)

