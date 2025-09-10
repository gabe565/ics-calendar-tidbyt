FROM gcr.io/distroless/static:nonroot
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/ics-calendar-tidbyt /
ENTRYPOINT ["/ics-calendar-tidbyt"]
