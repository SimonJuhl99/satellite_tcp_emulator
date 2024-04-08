import pandas as pd
from matplotlib import pyplot as plt
import numpy as np
import time

metrics_info = {
    "window_scale_send"                 : { "type" : "none" ,       "ylim" : (0,999999999) }, 
    "window_scale_receive"              : { "type" : "none" ,       "ylim" : (0,999999999) },
    "retransmission_timeout"            : { "type" : "none" ,       "ylim" : (0,999999999) },
    "rtt_mean"                          : { "type" : "sample" ,     "ylim" : (0, 300) },
    "rtt_var"                           : { "type" : "sample" ,     "ylim" : (0,999999999) },
    "acknowledgement_timeout"           : { "type" : "none" ,       "ylim" : (0,999999999) }, 
    "path_mtu"                          : { "type" : "none" ,       "ylim" : (0,999999999) }, 
    "receive_maximum_segment_size"      : { "type" : "none" ,       "ylim" : (0,999999999) },
    "advertised_maximum_segment_size"   : { "type" : "none" ,       "ylim" : (0,999999999) }, 
    "maximum_segment_size"              : { "type" : "none" ,       "ylim" : (0,999999999) },
    "congestion_window"                 : { "type" : "sample" ,     "ylim" : (0, 4000) },
    "bytes_sent"                        : { "type" : "cumulative" , "ylim" : (0, 1600000) },
    "bytes_acked"                       : { "type" : "cumulative" , "ylim" : (0,999999999) },
    "bytes_received"                    : { "type" : "cumulative" , "ylim" : (0, 1600000) },
    "bytes_retrans"                     : { "type" : "none" ,       "ylim" : (0,999999999) },
    "segments_out"                      : { "type" : "none" ,       "ylim" : (0,999999999) },
    "segments_in"                       : { "type" : "none" ,       "ylim" : (0,999999999) },
    "data_segments_out"                 : { "type" : "none" ,       "ylim" : (0,999999999) },
    "data_segments_in"                  : { "type" : "none" ,       "ylim" : (0,999999999) },
    "send_rate"                         : { "type" : "none" ,       "ylim" : (0,999999999) },
    "last_send"                         : { "type" : "none" ,       "ylim" : (0,999999999) },
    "last_receive"                      : { "type" : "none" ,       "ylim" : (0,999999999) },
    "last_acknowledgment"               : { "type" : "sample" ,     "ylim" : (0,5000) },
    "pacing_rate"                       : { "type" : "sample" ,     "ylim" : (0,200000) },
    "delivery_rate"                     : { "type" : "sample" ,     "ylim" : (0,2000000) },
    "delivered"                         : { "type" : "none" ,       "ylim" : (0,999999999) },
    "busy"                              : { "type" : "none" ,       "ylim" : (0,999999999) },
    "receive_space"                     : { "type" : "none" ,       "ylim" : (0,999999999) },
    "receive_slow_start_threshold"      : { "type" : "none" ,       "ylim" : (0,999999999) },
    "minimum_rtt"                       : { "type" : "none" ,       "ylim" : (0,999999999) },
    "send_window"                       : { "type" : "none" ,       "ylim" : (0,999999999) }, 
    "receive_rtt"                       : { "type" : "none" ,       "ylim" : (0,999999999) }, 
    "application_limited"               : { "type" : "none" ,       "ylim" : (0,999999999) }, 
    "backoff"                           : { "type" : "none" ,       "ylim" : (0,999999999) },
    "unacked"                           : { "type" : "sample" ,     "ylim" : (0,2000) },
    "dsack_dups"                        : { "type" : "sample" ,     "ylim" : (0,2000) }, 
    "lost"                              : { "type" : "none" ,       "ylim" : (0,999999999) }, 
    "retrans"                           : { "type" : "sample" ,     "ylim" : (0,2000) }, 
    "retrans_total"                     : { "type" : "none" ,       "ylim" : (0,999999999) }
}

def loadFile(file):
    route_change_times = []
    str_route_change_times = []

    content=file.readlines()
    #content.pop(0)              # do not include the first time a route is found

    for line in content:
        spl = line.split()
        for idx, word in enumerate(spl):
            if word == "time":
                route_change_times.append(int(spl[idx+1]))
                str_route_change_times.append(spl[idx+1])
                break

    print(route_change_times)
    file.close()

    return [route_change_times, str_route_change_times]

# convert a cumulative metric to period
def accumulated_to_periodic(df, id, metric):
    prev_accumulated = -1
    y = []

    for i, el in enumerate(df['timestamp']):
        # the ingoing connection has id=1
        if int(df['id'][i]) == id:
            if prev_accumulated == -1:
                prev_accumulated = df[metric][i]
                continue    # continue since prev_accumulated will make the goodput 0 for this iteration
            
            periodic_value = df[metric][i] - prev_accumulated
            y.append(periodic_value)   
            prev_accumulated = df[metric][i]

    return y


def receiver_handler(df, id, metrics):
    num_of_cumulative_metrics = 0
    num_of_sampled_metrics = 0
    cumulative_metrics = []
    sampled_metrics = []

    for metric in metrics:
        if metrics_info[metric]["type"] == "sample":
            num_of_sampled_metrics += 1
            sampled_metrics.append(metric)
        elif metrics_info[metric]["type"] == "cumulative":
            num_of_cumulative_metrics += 1
            cumulative_metrics.append(metric)
        else:
            print("Type of metric '" + metric + "' is unknown")
            return 0

    start_time = df['timestamp'][0].to_pydatetime()
    unix_start_time = time.mktime(start_time.timetuple())*1e3+start_time.microsecond/1e3
    first_iteration = True

    x = []
    y = [[] for i in range(len(metrics))]     # 1000 seconds with frequency 50Hz

    for i, el in enumerate(df['timestamp']):
        # the ingoing connection has id=1
        if int(df['id'][i]) == id:
            if first_iteration:
                first_iteration = False
                continue    # since some metrics are accumulated values we cannot use just the value of the first iteration - therefore we skip the first
            time_string = df['timestamp'][i].to_pydatetime()
            unix_time_stamp = time.mktime(time_string.timetuple())*1e3+time_string.microsecond/1e3
            x.append(float((unix_time_stamp - unix_start_time)/1000))
            for j, metric in enumerate(sampled_metrics):
                y[j].append(df[metric][i]) 

    for i, metric in enumerate(cumulative_metrics):
        y[num_of_sampled_metrics + i] = accumulated_to_periodic(df, id, metric)
        
    
    return np.array(x), np.array(y), unix_start_time


def sender_handler(df, id, metrics):
    num_of_cumulative_metrics = 0
    num_of_sampled_metrics = 0
    cumulative_metrics = []    
    sampled_metrics = []
    metrics_order = []

    for metric in metrics:
        if metrics_info[metric]["type"] == "sample":
            num_of_sampled_metrics += 1
            sampled_metrics.append(metric)
        elif metrics_info[metric]["type"] == "cumulative":
            num_of_cumulative_metrics += 1
            cumulative_metrics.append(metric)
        else:
            print("Type of metric '" + metric + "' is unknown")
            return 0

    start_time = df['timestamp'][0].to_pydatetime()
    unix_start_time = time.mktime(start_time.timetuple())*1e3+start_time.microsecond/1e3
    first_iteration = True 

    x = []
    y = [[] for i in range(len(metrics))]     # 1000 seconds with frequency 50Hz

    for i, el in enumerate(df['timestamp']):
        # the ingoing connection has id=1
        if int(df['id'][i]) == id:
            if first_iteration:
                first_iteration = False
                continue    # since some metrics are accumulated values we cannot use just the value of the first iteration - therefore we skip the first
            time_string = df['timestamp'][i].to_pydatetime()
            unix_time_stamp = time.mktime(time_string.timetuple())*1e3+time_string.microsecond/1e3
            x.append(float((unix_time_stamp - unix_start_time)/1000))
            for j, metric in enumerate(sampled_metrics):
                y[j].append(df[metric][i]) 

    for i, metric in enumerate(cumulative_metrics):
        y[num_of_sampled_metrics + i] = accumulated_to_periodic(df, id, metric)

    #print(df.columns)

    return np.array(x), np.array(y), unix_start_time


def start_time_normalization(receiver_start_time, sender_start_time, receiver_t, sender_t):
    new_receiver_t = receiver_t
    new_sender_t = sender_t

    if int(receiver_start_time - sender_start_time) == 0:
        pass
    elif int(receiver_start_time - sender_start_time) < 0:          # receiver events are ahead of sender events
        # add the time-difference which synchronizes the events
        new_receiver_t = receiver_t + (abs(receiver_start_time - sender_start_time) / 1000)
    elif int(receiver_start_time - sender_start_time) > 0:          # sender events are ahead of receiver events
        # add the time-difference which synchronizes the events
        new_sender_t = sender_t + (abs(receiver_start_time - sender_start_time) / 1000)

    return new_receiver_t, new_sender_t

def plotty(cubic_d, reno_d):
    fig, (ax1, ax2) = plt.subplots(2, figsize=(24,12))
    receiver_t, receiver_data, receiver_labels, sender_t, sender_data, sender_labels = [0]*6

    fig.subplots_adjust(right=0.85)

    axes_top = []
    axes_bottom = []

    for i, d in enumerate([cubic_d, reno_d]):
        if d == []:
            continue
        receiver_t, receiver_data, receiver_labels, sender_t, sender_data, sender_labels = d
        axes = []
        num_of_metrics = len(receiver_data) + len(sender_data)
        if (num_of_metrics > 4) and (num_of_metrics < 1):
            print("ERR: Number of metrics new to be between 1-4")

        if i == 0:
            axes_top.append(ax1)
            for _ in range(num_of_metrics - 1):            # minus 1 since minimum number of metrics is 1 
                axes_top.append(ax1.twinx())
            axes = axes_top
        elif i == 1:
            axes_bottom.append(ax2)
            for _ in range(num_of_metrics - 1):            # minus 1 since minimum number of metrics is 1 
                axes_bottom.append(ax2.twinx())
            axes = axes_bottom
            
        for i in range(max(0, num_of_metrics - 2)):
            axes[i+2].spines.right.set_position(("axes", 1 + 0.05*(i+1)))

        ps = []

        for j, metric in enumerate(receiver_data):
            p, = axes[j].plot(np.asarray(receiver_t, float), metric, "C"+str(j), label=receiver_labels[j])
            ps.append(p)

        for j, metric in enumerate(sender_data):
            p, = axes[len(receiver_data)+j].plot(np.asarray(sender_t, float), metric, "C"+str(len(receiver_data)+j), label=sender_labels[j])
            ps.append(p)

        labels = receiver_labels + sender_labels        

        axes[0].set(xlim=(-10, 1010), ylim=metrics_info[labels[0]]['ylim'], xlabel="Time (seconds)", ylabel=labels[0])
        for j in range(num_of_metrics-1):
            axes[j+1].set(ylim=metrics_info[labels[j+1]]['ylim'], ylabel=labels[j+1])

        for j, metric in enumerate(receiver_data):
            axes[j].yaxis.label.set_color(          ps[j].get_color())
            axes[j].tick_params(axis='y', colors=   ps[j].get_color())

        for j, metric in enumerate(sender_data):
            axes[len(receiver_data)+j].yaxis.label.set_color(       ps[len(receiver_data)+j].get_color())
            axes[len(receiver_data)+j].tick_params(axis='y', colors=ps[len(receiver_data)+j].get_color())

        axes[num_of_metrics-1].legend(handles=ps, loc='upper right')
        
    plt.show()


def setup1():
    # IMPORTANT! Put metrics with "type:sample" first in each array
    #bytes_received
    #congestion_window

    receiver_metrics = [] #["last_acknowledgment"]
    sender_metrics = ["rtt_mean", "congestion_window"]

    receiver_metrics = ["bytes_received"] #["last_acknowledgment"]
    sender_metrics = ["rtt_mean", "congestion_window"]

    data_test1 = ["./tcp_stats/tcp_stats_koto_cubic.parquet", 
                  "./tcp_stats/tcp_stats_elalamo_cubic.parquet"]
    data_test2 = ["./tcp_stats/tcp_stats_koto_reno.parquet", 
                  "./tcp_stats/tcp_stats_elalamo_reno.parquet"]

    ids_test1 = [0,1]
    ids_test2 = [1,1]

    top_bottom_comparison(receiver_metrics, sender_metrics, data_test1, data_test2, ids_test1, ids_test2)

def setup2():
    total_time = 1000

    metrics = ["rtt_mean", "congestion_window"]
    #metrics = ["bytes_received"]
    df = pd.read_parquet('./tcp_stats/drop_vs_nodrop/tcp_stats_elalamo_cubic_drop.parquet', engine='fastparquet')
    t, data, _ = sender_handler(df, 1, metrics)

    fig, ax = plt.subplots(figsize=(24,12))

    fig.subplots_adjust(right=0.85)

    axes = []
    num_of_metrics = len(data)
    if (num_of_metrics > 4) and (num_of_metrics < 1):
        print("ERR: Number of metrics new to be between 1-4")

    axes.append(ax)
    for _ in range(num_of_metrics - 1):            # minus 1 since minimum number of metrics is 1 
        axes.append(ax.twinx())
    
    for i in range(max(0, num_of_metrics - 2)):
        axes[i+2].spines.right.set_position(("axes", 1 + 0.05*(i+1)))

    ps = []

    for j, metric in enumerate(data):
        p, = axes[j].plot(np.asarray(t, float), metric, "C"+str(j), label=metrics[j])
        ps.append(p)

    axes[0].set(xlim=(-1, total_time+1), ylim=metrics_info[metrics[0]]['ylim'], xlabel="Time (seconds)", ylabel=metrics[0])
    for j in range(num_of_metrics-1):
        axes[j+1].set(ylim=metrics_info[metrics[j+1]]['ylim'], ylabel=metrics[j+1])

    for j, metric in enumerate(data):
        axes[j].yaxis.label.set_color(          ps[j].get_color())
        axes[j].tick_params(axis='y', colors=   ps[j].get_color())

    axes[num_of_metrics-1].legend(handles=ps, loc='upper right')
        
    plt.show()

def reno_old_vs_new():
    # IMPORTANT! Put metrics with "type:sample" first in each array
    #bytes_received
    #congestion_window

    receiver_metrics = ["bytes_received"] #["last_acknowledgment"]
    sender_metrics = ["rtt_mean", "congestion_window"]

    data_test1 = ["./tcp_stats/drop_vs_nodrop/tcp_stats_koto_reno_drop.parquet", 
                  "./tcp_stats/drop_vs_nodrop/tcp_stats_elalamo_reno_drop.parquet"]
    data_test2 = ["./tcp_stats/drop_vs_nodrop/tcp_stats_koto_reno_nodrop.parquet", 
                  "./tcp_stats/drop_vs_nodrop/tcp_stats_elalamo_reno_nodrop.parquet"]
    
    # [receiver_flow_id, sender_flow_id]
    ids_test1 = [0,1]
    ids_test2 = [0,0]

    top_bottom_comparison(receiver_metrics, sender_metrics, data_test1, data_test2, ids_test1, ids_test2)

def cubic_old_vs_new():
    # IMPORTANT! Put metrics with "type:sample" first in each array
    #bytes_received
    #congestion_window

    receiver_metrics = ["bytes_received"] #["last_acknowledgment"]
    sender_metrics = ["rtt_mean", "congestion_window"]

    data_test1 = ["./tcp_stats/drop_vs_nodrop/tcp_stats_koto_cubic_drop.parquet", 
                  "./tcp_stats/drop_vs_nodrop/tcp_stats_elalamo_cubic_drop.parquet"]
    data_test2 = ["./tcp_stats/drop_vs_nodrop/tcp_stats_koto_cubic_nodrop.parquet", 
                  "./tcp_stats/drop_vs_nodrop/tcp_stats_elalamo_cubic_nodrop.parquet"]

    ids_test1 = [0,1]
    ids_test2 = [0,0]

    top_bottom_comparison(receiver_metrics, sender_metrics, data_test1, data_test2, ids_test1, ids_test2)

def top_bottom_comparison(receiver_metrics, sender_metrics, data_test1, data_test2, ids_test1, ids_test2):
    receiver_df = pd.read_parquet(data_test1[0], engine='fastparquet')
    receiver_t, receiver_data, receiver_start_time = receiver_handler(receiver_df, ids_test1[0], receiver_metrics)
    sender_df = pd.read_parquet(data_test1[1], engine='fastparquet')
    sender_t, sender_data, sender_start_time = sender_handler(sender_df, ids_test1[1], sender_metrics)
    receiver_t, sender_t = start_time_normalization(receiver_start_time, sender_start_time, receiver_t, sender_t)
    
    cubic_nodrop_data = [receiver_t, receiver_data, receiver_metrics, sender_t, sender_data, sender_metrics]

    receiver_df = pd.read_parquet(data_test2[0], engine='fastparquet')
    receiver_t, receiver_data, receiver_start_time = receiver_handler(receiver_df, ids_test2[0], receiver_metrics)
    sender_df = pd.read_parquet(data_test2[1], engine='fastparquet')
    sender_t, sender_data, sender_start_time = sender_handler(sender_df, ids_test2[1], sender_metrics)
    receiver_t, sender_t = start_time_normalization(receiver_start_time, sender_start_time, receiver_t, sender_t)

    cubic_data = [receiver_t, receiver_data, receiver_metrics, sender_t, sender_data, sender_metrics]

    plotty(cubic_nodrop_data, cubic_data)

if __name__ == "__main__":
    #cubic_old_vs_new()
    #reno_old_vs_new()
    #setup1()
    f = open("./route_change_files/route-changes-update-L3-every-30-seconds", "r")
    loadFile(f)
    

