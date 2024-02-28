import matplotlib.pyplot as plt
import numpy as np

file1 = open("/tmp/route-changes-kepler", "r")
file2 = open("/tmp/route-changes-oneweb", "r")
file3 = open("/tmp/route-changes-starlink", "r")
file_arr = [file1, file2, file3]


def loadFile(files):
    route_change_times = []
    str_route_change_times = []

    for file in files:
        tmp_rct = []
        tmp_str_rct = []

        content=file.readlines()
        content.pop(0)              # do not include the first time a route is found

        for line in content:
            spl = line.split()
            for idx, word in enumerate(spl):
                if word == "time":
                    tmp_rct.append(int(spl[idx+1]))
                    tmp_str_rct.append(spl[idx+1])
                    break

        route_change_times.append(tmp_rct)
        str_route_change_times.append(tmp_str_rct)
        
        file.close()

    return [route_change_times, str_route_change_times]


def makeTimelinePlot(route_change_times, str_route_change_times):
    
    fig, axs = plt.subplots(3, figsize=(20, 7), tight_layout="True")
    

    for sat_constellation in range(len(route_change_times)):
        data = route_change_times[sat_constellation]
        str_data = str_route_change_times[sat_constellation]


        levels = np.tile([-2, 2, -1, 1], int(np.ceil(len(data)/4)))[:len(data)]

        axs[sat_constellation].vlines(data, 0, levels, color="tab:red")

        axs[sat_constellation].plot(data, np.zeros_like(data), "-o",
                color="k", markerfacecolor="w")

        for d, l, r in zip(data, levels, str_data):
            axs[sat_constellation].annotate(r, xy=(d, l),
                        xytext=(-3, np.sign(l)*3), textcoords="offset points",
                        horizontalalignment="right",
                        verticalalignment="bottom" if l > 0 else "top")
            
        axs[sat_constellation].yaxis.set_visible(False)
        axs[sat_constellation].xaxis.set_visible(False)
        axs[sat_constellation].spines[["left", "top", "right"]].set_visible(False)
    plt.show()

def makeChangeFreqPlot(route_change_times, mode):
    
    fig, ax = plt.subplots()

    number_of_changes = []                  # during 1000 seconds
    avg_interval_between_changes = []
    
    for sat_constellation in range(len(route_change_times)):
        number_of_changes.append(len(route_change_times[sat_constellation]))
        avg_interval_between_changes.append(round(1000/len(route_change_times[sat_constellation]), 1))

    constellations = ['Kepler', 'OneWeb', 'Starlink']
    bar_labels = ['red', 'blue', 'orange']
    bar_colors = ['tab:red', 'tab:blue', 'tab:orange']

    if mode == "change_interval":
        bars = ax.bar(constellations, avg_interval_between_changes, label=bar_labels, color=bar_colors)
        ax.set_title('Average time between route changes')
    elif mode == "change_number":
        bars = ax.bar(constellations, number_of_changes, label=bar_labels, color=bar_colors)
        ax.set_ylabel('Route changes')
        ax.set_title('Number of route changes during 1000 seconds')
    else:
        print("ERR: not a mode")
        exit()

    for bar in bars:
        height = bar.get_height()
        ax.annotate('{}'.format(height),
                    xy=(bar.get_x() + bar.get_width() / 2, height),
                    xytext=(0, 3),  # 3 points vertical offset
                    textcoords="offset points",
                    ha='center', va='bottom')
    plt.show()

if __name__ == '__main__':
    route_change_times, str_route_change_times = loadFile(file_arr)
    makeTimelinePlot(route_change_times, str_route_change_times)
    makeChangeFreqPlot(route_change_times, "change_interval")
    makeChangeFreqPlot(route_change_times, "change_number")
