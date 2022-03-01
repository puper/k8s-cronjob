# kubernetes cronjob [developing]
- Create CronJob use this image  to run command in other existing pod.
- use labels to select one running pod.
- use pod name to select pod exactly.

## usage:
- /app/k8s-cronjob -pn podName -cn containerName your command here
- /app/k8s-cronjob -l labelSeletors -cn containerName your command here