# Generated by the gRPC Python protocol compiler plugin. DO NOT EDIT!
"""Client and server classes corresponding to protobuf-defined services."""
import grpc

import deduplicator_pb2 as deduplicator__pb2


class DeduplicatorStub(object):
    """Deduplicator is a micro service that calculate the SimHash for a given text.
    """

    def __init__(self, channel):
        """Constructor.

        Args:
            channel: A grpc.Channel.
        """
        self.GetSimHash = channel.unary_unary(
                '/protocol.Deduplicator/GetSimHash',
                request_serializer=deduplicator__pb2.GetSimHashRequest.SerializeToString,
                response_deserializer=deduplicator__pb2.GetSimHashResponse.FromString,
                )


class DeduplicatorServicer(object):
    """Deduplicator is a micro service that calculate the SimHash for a given text.
    """

    def GetSimHash(self, request, context):
        """Obtains the similarity hash for the incoming text.
        """
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')


def add_DeduplicatorServicer_to_server(servicer, server):
    rpc_method_handlers = {
            'GetSimHash': grpc.unary_unary_rpc_method_handler(
                    servicer.GetSimHash,
                    request_deserializer=deduplicator__pb2.GetSimHashRequest.FromString,
                    response_serializer=deduplicator__pb2.GetSimHashResponse.SerializeToString,
            ),
    }
    generic_handler = grpc.method_handlers_generic_handler(
            'protocol.Deduplicator', rpc_method_handlers)
    server.add_generic_rpc_handlers((generic_handler,))


 # This class is part of an EXPERIMENTAL API.
class Deduplicator(object):
    """Deduplicator is a micro service that calculate the SimHash for a given text.
    """

    @staticmethod
    def GetSimHash(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(request, target, '/protocol.Deduplicator/GetSimHash',
            deduplicator__pb2.GetSimHashRequest.SerializeToString,
            deduplicator__pb2.GetSimHashResponse.FromString,
            options, channel_credentials,
            insecure, call_credentials, compression, wait_for_ready, timeout, metadata)
