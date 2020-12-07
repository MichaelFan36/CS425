

import numpy as np
import matplotlib.pyplot as plt
reader = open("/Users/changkehang/Desktop/ECE428-spring2020/MP/Wenqian_MP2Part1/log(1)(100).txt", "r")

bandwidth = {}
transactions = {}

bandwidthMaxTime = 0
bandwidthMinTime = float("inf")

while 1:
    logLine = reader.readline()
    if logLine == "" or logLine == "\n" or logLine is None:
        break
    log = logLine.split(' ')
    if log[0] == "B":
        time = int(float(log[1]))
        length = int(float(log[2]))
        if time > bandwidthMaxTime:
            bandwidthMaxTime=time
        if time < bandwidthMinTime:
            bandwidthMinTime = time
        bandwidth[time] = length
    elif log[0] == "T":
        id = log[4]
        createTime = float(log[4])
        maxPropagationTime = 0
        receiveNumber = 0
        if (id in transactions):
            maxPropagationTime, receiveNumber, createTime = transactions[id]

        propDelay = float(log[2]) - createTime
        maxPropagationTime = max(propDelay, maxPropagationTime)
        receiveNumber += 1
        transactions[id] = (maxPropagationTime, receiveNumber, createTime)


bandwidthDat = np.zeros(bandwidthMaxTime-bandwidthMinTime+1)
for k,v in bandwidth.items():
    bandwidthDat[k-bandwidthMinTime] = v

plt.plot(bandwidthDat)
plt.title("BW (100 nodes) in 120s (thanos after 60s)")
plt.xlabel("T(s)")
plt.ylabel("BW(BYTES)")
plt.show()


transactions = list(transactions.values())
transactions = sorted(transactions, key=lambda x:x[2])
transactionDat = np.zeros((0,3))
for transaction in transactions:
    transactionDat = np.append(transactionDat, [[transaction[0], transaction[1], transaction[2]]], axis = 0)
transactionDat[:,2] -= np.min(transactionDat[:,2])



plt.scatter(transactionDat[:,2], transactionDat[:,1])
plt.title("Reached nodes for 100 nodes in 120s (thanos after 60s)")
plt.xlabel("Transaction create time(second)")
plt.ylabel("Number of nodes received")
plt.show()
plt.scatter(transactionDat[:,2], transactionDat[:,0])
plt.title("Propagation time for 100 nodes in 120s (thanos after 60s)")
plt.xlabel("Transaction create time(second)")
plt.ylabel("Propagation time(second)")
plt.show()
