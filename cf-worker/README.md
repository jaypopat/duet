This typescript module contains code for a cf worker and a cf agent

- The CF worker acts as a load balancer which takes requests and distributes them to the agent/durable object. (we have one agent per room).

- The agent uses a llama model to answer queries, also taking the last 20 messages of the state as context for inference

- We also have a sandbox configured for the env, which we will utilise later - one sandbox per room with the agent being able to run commands in the sandbox session.