all:
	+$(MAKE) -C grpc/client
	+$(MAKE) -C grpc/server
	+$(MAKE) -C micro/client
	+$(MAKE) -C micro/server
