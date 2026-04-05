# Bugs found or other changes
## Backup plans not showing "started" status
- not resolved
- nice to have #v2

When starting a new job we can't see that we triggered it. Ideally I would like to be able to see when a job is running as long as the server and agent are able to communicate.

* When a job is ran manually, we should immediatly add it as "started" or better yet "planned".
* When a job is started on the agent either manually or scheduled the agent should let the server know that the job has started

## Limited information when something goes wrong
- not resolved
- Must have #v1

When a job fails there is extremely limited information on what went wrong, I can neither see anyting in the agent logs or on the dashboard.

Ideally I would like to see why it went wrong on the dashboard.
I would like to see both our internal logs, what steps we did and where it went wrong.
But also logs from restic if that was the cause.

Most important is to see why something went wrong, but I think it can be helpful to see logs for each job even if it went successful.

## Rethink the design
- not resolved
- future ideas #v3/v4

The dashboard looks simple but also quite "old style" and not modern.

Find inspiration from backrest but also more modern dashboards.

## For some reason the server tends tto stop working
- not resolved
- Must be fixed

For some reason the server seems to occasionally stop and I can't see anything in the logs.

Server logs:
❯ pod logs docker_server_1 -f
2026/04/04 22:42:38 main.go:32: Database opened at /data/server.db
2026/04/04 22:42:38 main.go:56: gRPC server listening on :8443
2026/04/04 22:42:38 main.go:64: HTTP server listening on :8080
2026/04/04 22:42:41 connect.go:50: Agent d5acf899-6be2-414e-953f-a2aaa84a8621 (demo-agent) connected, status=approved
2026/04/04 22:42:54 "GET http://localhost:8080/api/agents HTTP/1.1" from 10.89.0.1:44998 - 200 502B in 613.776µs
2026/04/04 22:42:54 "GET http://localhost:8080/api/scripts HTTP/1.1" from 10.89.0.1:45012 - 200 3B in 117.644µs
2026/04/04 22:42:54 "GET http://localhost:8080/api/plans/ec0a6342-a195-4bc4-b572-eaf30915ae9d HTTP/1.1" from 10.89.0.1:44992 - 200 399B in 11.320986ms
2026/04/04 22:42:54 "GET http://localhost:8080/api/jobs?plan_id=ec0a6342-a195-4bc4-b572-eaf30915ae9d HTTP/1.1" from 10.89.0.1:44992 - 200 2140B in 381.278µs
2026/04/04 22:42:54 "GET http://localhost:8080/api/plans/ec0a6342-a195-4bc4-b572-eaf30915ae9d/hooks HTTP/1.1" from 10.89.0.1:45012 - 200 3B in 373.916µs
2026/04/04 22:42:55 "GET http://localhost:8080/plans/ec0a6342-a195-4bc4-b572-eaf30915ae9d HTTP/1.1" from 10.89.0.1:45012 - 200 544B in 34.655µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/index-B0zPLOma.js HTTP/1.1" from 10.89.0.1:45012 - 200 50585B in 15.798511ms
2026/04/04 22:42:55 "GET http://localhost:8080/assets/pinia-Da-KYebi.js HTTP/1.1" from 10.89.0.1:44992 - 200 68312B in 15.010483ms
2026/04/04 22:42:55 "GET http://localhost:8080/assets/index-BfbVb6R3.css HTTP/1.1" from 10.89.0.1:44998 - 200 25050B in 15.100202ms
2026/04/04 22:42:55 "GET http://localhost:8080/assets/PlanDetailView-C0piJsSR.js HTTP/1.1" from 10.89.0.1:44998 - 200 13358B in 142.17µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/ConfirmDialog-BGtNrojb.js HTTP/1.1" from 10.89.0.1:44992 - 200 1429B in 69.912µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/LoadingSpinner-D7q3nthw.js HTTP/1.1" from 10.89.0.1:45012 - 200 3190B in 61.161µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/DataTable-K_KEHFdM.js HTTP/1.1" from 10.89.0.1:45016 - 200 3199B in 124.737µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/StatusBadge-B1lb0I11.js HTTP/1.1" from 10.89.0.1:45012 - 200 720B in 42.688µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/agents-BUcIC1r9.js HTTP/1.1" from 10.89.0.1:45016 - 200 1142B in 64.884µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/jobs-wuGNb7Ei.js HTTP/1.1" from 10.89.0.1:44992 - 200 533B in 22.056µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/plans-GG4EexDH.js HTTP/1.1" from 10.89.0.1:44998 - 200 1213B in 38.149µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/time-CROq44dE.js HTTP/1.1" from 10.89.0.1:44992 - 200 939B in 64.139µs
2026/04/04 22:42:55 "GET http://localhost:8080/assets/scripts-D5X3n9AC.js HTTP/1.1" from 10.89.0.1:45016 - 200 1090B in 63.671µs
2026/04/04 22:42:55 "GET http://localhost:8080/api/agents HTTP/1.1" from 10.89.0.1:45016 - 200 502B in 271.713µs
2026/04/04 22:42:55 "GET http://localhost:8080/api/plans/ec0a6342-a195-4bc4-b572-eaf30915ae9d HTTP/1.1" from 10.89.0.1:44992 - 200 399B in 513.546µs
2026/04/04 22:42:55 "GET http://localhost:8080/favicon.svg HTTP/1.1" from 10.89.0.1:45012 - 200 9522B in 138.886µs
2026/04/04 22:42:55 "GET http://localhost:8080/api/scripts HTTP/1.1" from 10.89.0.1:44998 - 200 3B in 93.604µs
2026/04/04 22:42:55 "GET http://localhost:8080/api/plans/ec0a6342-a195-4bc4-b572-eaf30915ae9d/hooks HTTP/1.1" from 10.89.0.1:44998 - 200 3B in 128.336µs
2026/04/04 22:42:55 "GET http://localhost:8080/api/jobs?plan_id=ec0a6342-a195-4bc4-b572-eaf30915ae9d HTTP/1.1" from 10.89.0.1:45012 - 200 2140B in 365.723µs
2026/04/04 22:42:56 "POST http://localhost:8080/api/plans/ec0a6342-a195-4bc4-b572-eaf30915ae9d/trigger HTTP/1.1" from 10.89.0.1:44998 - 200 28B in 1.673163ms
2026/04/04 22:43:41 connect.go:98: Agent d5acf899-6be2-414e-953f-a2aaa84a8621 disconnected

agent logs:
time=2026-04-04T22:42:40.861Z level=INFO msg="agent starting" source=agent agent_name=demo-agent server=server:8443 data_dir=/data
time=2026-04-04T22:42:40.864Z level=INFO msg="loaded identity" source=agent agent_id=d5acf899-6be2-414e-953f-a2aaa84a8621
time=2026-04-04T22:42:40.864Z level=INFO msg="starting scheduler with local config" source=agent config_version=2
time=2026-04-04T22:42:40.864Z level=INFO msg="scheduled plan" source=scheduler plan=test cron="0 2 * * *"
time=2026-04-04T22:42:40.864Z level=INFO msg="connecting to server..." source=agent
time=2026-04-04T22:42:56.794Z level=INFO msg="received command" source=stream command_id=035d2569-9665-4f23-b985-289d4fe4fb89
time=2026-04-04T22:42:56.794Z level=INFO msg="manual trigger for plan" source=scheduler plan=test
time=2026-04-04T22:42:56.794Z level=INFO msg="starting backup job" source=orchestrator job_id=9f4896c7-b96d-422b-9114-f5b7ad40c7b8 plan=test trigger=manual
time=2026-04-04T22:42:56.794Z level=INFO msg="backing up to repository" source=orchestrator repository=local path=/tmp
time=2026-04-04T22:42:56.794Z level=INFO msg="ensuring repository is initialized" source=restic repository=local
time=2026-04-04T22:42:57.568Z level=INFO msg="running restic backup" source=restic repository=local
time=2026-04-04T22:43:52.924Z level=INFO msg="backup succeeded" source=restic repository=local snapshot_id=e87cdb2e0e5d812f8891790d04a57fae5a46a283907e739c7c7c3ac13f9c4353 files_new=160534 files_changed=0 bytes_added=2757674538
time=2026-04-04T22:43:52.925Z level=INFO msg="backup job completed" source=orchestrator job_id=9f4896c7-b96d-422b-9114-f5b7ad40c7b8 status=success duration=56.131216472s
time=2026-04-04T22:44:02.941Z level=WARN msg="direct report delivery failed, buffering" source=agent error="report job RPC: rpc error: code = DeadlineExceeded desc = context deadline exceeded"
time=2026-04-04T22:44:40.897Z level=INFO msg="flushing buffered reports" source=reporter count=1

Browser just says "ERR_EMPTY_RESPONSE"

## Agents page is "gone"
- not resolved
- nice to have #v2

With the new redesign of the frontend I noticed the approval of new agents is a bit weird.
When a new agent needs to be approved we can see a notification about it on the fleet-overview, we can see it "pending".

However it's a bit confusing where to accept it, from the looks of it to accept it we have to click "review" on the notification which brings us to some /agents page which I can only get to this way, it's not in the sidebar or anything.

Ideally what I would like to see is being able to accept/deny it by going directly to the pending agent from fleet-overview.
This makes most sense.

We could keep the "review" as the more agents I add we might want just a plain list of agents too.
But in that case it should be exposed on the sidebar.

## Agents remote path
- not resolved
- nice to have #v2/v3

When creating paths related to a specific agent it would be nice to get autocomplete or remote "browser" suggestion correct paths.

For example wwhen creating a repository bound to a agent or adding a backup plan, it would be nice if some type of path explorer appeared so that we could traverse through the agent's folder to get the correct path.

Minimizes potential typos and also makes configuration easier.

This would presumably require some new APIs between server and agent.

## More "live" data
- not resolved
- nice to have #v2/v3

When new information comes into the server such as job data it should be pushed/streamed to the client.
Ideally we should use something like WebRTC on the frontend-server.

So if I am standing on the job list, trigger a backup I will want to see live updates on what is going on.

This plays along with ## Backup plans not showing "started" status
If we have that task, this will be even more important

## GRPC/HTTP migration
- not resolve
- future ideas #v3/v4

It would be nice to minimize the amount of ports exposed, ideally I would only want to expose 1 port so an idea is to look into how to combine HTTP with GRPC.
I got the suggestio nthat some multiplexer like cmux might work?
Alterenatively maybe a prroxy inside a docker could work but then we can't use minimal containers I guess.

When asked the reasoning for 2 ports I got totld gRPC uses HTTP/2 and not HTTTP/1.1, is there a downside to forcing HTTP/2?

The main reason is that I am exposing the server through tailscale and would like to expose only 1 port as a service, but solutions can be worked around.
Another option would be that this should be handled on the other side (user/control side), so in my case in the homelab 

## More detailed logs
- not resolved
- nice to have #v2/v3

It would be nice to be able to see the actual restic commands being ran.
For example I got this error
```json
error:
restic snapshots check failed: exit status 1
stderr: Fatal: parsing repository location failed: invalid backend
If the repository is in a local directory, you need to add a `local:` prefix
```

And it would of helped if I knew what command arguments was passed on to understand this more.
Maybe passing on "environment" variables could be useful too.
Essentially showing the context that was used when the command was ran.

However keep in mind that some data could be sensitive, if we are passing on data such as Password we should probably skip/ignore them.
But otherwise it sounds like a good idea

## Accessibility - automatically open modals/navigation
- not resolve
- future ideas #v4

I noticed when clicking around that when I arrive on a list/table view such as a /agents /plans /repositories and such.
I often click on the row instead of "edit".

I think I like the idea of the buttons, but adding the functionality to also clicking on the row to interact with a main functionality.

Keep in mind that some functionality will override this, such as links or buttons.

It seems that /plans works this way but not /repositories.
So we should make this a consistent behavior and follow the /plans solution

## Syntax bug in repositories
- not resolved
- nice to have #v2/v3

when creating a repository that uses rclone I have to append rclone: to the repository path.

I would expect if I select the type "rclone" that this automatically get appended.
My assumption is that this behaviour is the same on the other types.

Preferably the solution is to append the syntax but an alternative would be to document this behaviour.