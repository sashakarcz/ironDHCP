import { useState, useEffect, useRef } from 'react';
import { Activity as ActivityIcon, Wifi, WifiOff, CheckCircle, XCircle, AlertCircle } from 'lucide-react';

interface ActivityEvent {
  id: string;
  timestamp: string;
  type: string;
  message: string;
  details?: {
    ip?: string;
    mac?: string;
    hostname?: string;
    subnet?: string;
    reason?: string;
    [key: string]: any;
  };
}

// Event type icons and colors
const eventConfig: Record<string, { icon: any; color: string; bgColor: string }> = {
  dhcp_discover: { icon: Wifi, color: 'text-blue-600', bgColor: 'bg-blue-50' },
  dhcp_offer: { icon: CheckCircle, color: 'text-green-600', bgColor: 'bg-green-50' },
  dhcp_request: { icon: Wifi, color: 'text-blue-600', bgColor: 'bg-blue-50' },
  dhcp_ack: { icon: CheckCircle, color: 'text-green-600', bgColor: 'bg-green-50' },
  dhcp_nak: { icon: XCircle, color: 'text-red-600', bgColor: 'bg-red-50' },
  dhcp_release: { icon: WifiOff, color: 'text-orange-600', bgColor: 'bg-orange-50' },
  dhcp_decline: { icon: AlertCircle, color: 'text-red-600', bgColor: 'bg-red-50' },
  git_sync: { icon: ActivityIcon, color: 'text-purple-600', bgColor: 'bg-purple-50' },
  connection: { icon: CheckCircle, color: 'text-gray-600', bgColor: 'bg-gray-50' },
};

export default function Activity() {
  const [events, setEvents] = useState<ActivityEvent[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const eventsEndRef = useRef<HTMLDivElement>(null);
  const eventsContainerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Connect to SSE endpoint
    const eventSource = new EventSource('/api/v1/activity/stream');

    eventSource.onopen = () => {
      console.log('SSE connection opened');
      setIsConnected(true);
    };

    eventSource.onerror = (error) => {
      console.error('SSE error:', error);
      setIsConnected(false);
    };

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        setEvents((prev) => [...prev.slice(-99), data]); // Keep last 100 events
      } catch (error) {
        console.error('Failed to parse event:', error);
      }
    };

    return () => {
      eventSource.close();
      setIsConnected(false);
    };
  }, []);

  // Auto-scroll to bottom when new events arrive
  useEffect(() => {
    if (autoScroll && eventsEndRef.current) {
      eventsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [events, autoScroll]);

  // Detect manual scroll to disable auto-scroll
  const handleScroll = () => {
    if (!eventsContainerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = eventsContainerRef.current;
    const isAtBottom = Math.abs(scrollHeight - scrollTop - clientHeight) < 10;
    setAutoScroll(isAtBottom);
  };

  const formatTimestamp = (timestamp: string) => {
    const date = new Date(timestamp);
    return date.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      fractionalSecondDigits: 3
    });
  };

  const formatEventType = (type: string) => {
    return type.replace(/_/g, ' ').toUpperCase();
  };

  return (
    <div>
      {/* Header */}
      <div className="mb-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-gray-900">Activity Log</h1>
            <p className="mt-2 text-gray-600">Real-time DHCP server events</p>
          </div>
          <div className="flex items-center gap-2">
            <div
              className={`flex items-center gap-2 px-4 py-2 rounded-lg ${
                isConnected ? 'bg-green-50 text-green-700' : 'bg-red-50 text-red-700'
              }`}
            >
              <div
                className={`w-2 h-2 rounded-full ${
                  isConnected ? 'bg-green-500 animate-pulse' : 'bg-red-500'
                }`}
              />
              <span className="text-sm font-medium">
                {isConnected ? 'Connected' : 'Disconnected'}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white p-4 rounded-lg shadow">
          <div className="text-2xl font-bold text-gray-900">{events.length}</div>
          <div className="text-sm text-gray-600">Total Events</div>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <div className="text-2xl font-bold text-blue-600">
            {events.filter(e => e.type === 'dhcp_discover').length}
          </div>
          <div className="text-sm text-gray-600">Discoveries</div>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <div className="text-2xl font-bold text-green-600">
            {events.filter(e => e.type === 'dhcp_ack').length}
          </div>
          <div className="text-sm text-gray-600">ACKs</div>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <div className="text-2xl font-bold text-orange-600">
            {events.filter(e => e.type === 'dhcp_release').length}
          </div>
          <div className="text-sm text-gray-600">Releases</div>
        </div>
      </div>

      {/* Events Feed */}
      <div className="bg-white rounded-lg shadow">
        <div className="px-6 py-4 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-gray-900">Live Events</h2>
          <button
            onClick={() => setAutoScroll(!autoScroll)}
            className={`px-3 py-1 text-sm rounded ${
              autoScroll
                ? 'bg-blue-100 text-blue-700'
                : 'bg-gray-100 text-gray-700'
            }`}
          >
            {autoScroll ? 'Auto-scroll ON' : 'Auto-scroll OFF'}
          </button>
        </div>
        <div
          ref={eventsContainerRef}
          onScroll={handleScroll}
          className="p-6 h-[600px] overflow-y-auto space-y-2"
        >
          {events.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-gray-400">
              <ActivityIcon className="w-16 h-16 mb-4" />
              <p className="text-lg">Waiting for events...</p>
              <p className="text-sm">Events will appear here in real-time</p>
            </div>
          ) : (
            events.map((event) => {
              const config = eventConfig[event.type] || eventConfig.connection;
              const Icon = config.icon;

              return (
                <div
                  key={event.id}
                  className={`flex items-start gap-3 p-3 rounded-lg ${config.bgColor} border border-gray-200`}
                >
                  <div className={`mt-1 ${config.color}`}>
                    <Icon className="w-5 h-5" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <span className={`text-xs font-semibold ${config.color}`}>
                        {formatEventType(event.type)}
                      </span>
                      <span className="text-xs text-gray-500">
                        {formatTimestamp(event.timestamp)}
                      </span>
                    </div>
                    <p className="text-sm text-gray-900 mb-1">{event.message}</p>
                    {event.details && (
                      <div className="flex flex-wrap gap-2 text-xs text-gray-600">
                        {event.details.ip && (
                          <span className="bg-white px-2 py-1 rounded">
                            IP: {event.details.ip}
                          </span>
                        )}
                        {event.details.mac && (
                          <span className="bg-white px-2 py-1 rounded">
                            MAC: {event.details.mac}
                          </span>
                        )}
                        {event.details.hostname && (
                          <span className="bg-white px-2 py-1 rounded">
                            Host: {event.details.hostname}
                          </span>
                        )}
                        {event.details.subnet && (
                          <span className="bg-white px-2 py-1 rounded">
                            Subnet: {event.details.subnet}
                          </span>
                        )}
                        {event.details.reason && (
                          <span className="bg-white px-2 py-1 rounded">
                            Reason: {event.details.reason}
                          </span>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              );
            })
          )}
          <div ref={eventsEndRef} />
        </div>
      </div>
    </div>
  );
}
