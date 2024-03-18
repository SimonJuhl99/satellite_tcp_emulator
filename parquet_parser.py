import pandas as pd
#df = pd.read_parquet('./emulator/satdata.parquet', engine='fastparquet')
#df = pd.read_parquet('/home/simon/Documents/tcp_stats_1.parquet', engine='fastparquet')
df = pd.read_parquet('./tcp_stats_real.parquet', engine='fastparquet')
#df = pd.read_parquet('./emulator/tcp_statistics6:27PM.parquet', engine='fastparquet')




print(df.columns)

for i in df['congestion_window']:
    print(i)