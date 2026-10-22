# Customer API source repository

This directory represents an independently versioned source repository. It is
referenced by `../fleck.yaml` together with the separate deployment repository.
The Dockerfile intentionally supplies `REDIS_ADRESS`, while the application
reads `REDIS_ADDRESS`.
