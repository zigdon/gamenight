import os
import sys
sys.path.append(os.path.join(os.path.dirname(__file__), 'lib'))

from apiclient.discovery import build
import datetime
import httplib2

class Utils:
    @classmethod
    def saturday(cls):
        """Return this week's saturday."""
        d = datetime.datetime.today().replace(hour=20, minute=0, second=0)
        weekday = d.weekday()
        if weekday == 6:
            return d + datetime.timedelta(days=6)
        elif weekday < 5:
            return d + datetime.timedelta(days=5-weekday)
        else:
            return d

    @classmethod
    def now(cls):
        """Return current datetime, adjusted for timezone."""
        now = datetime.datetime.now()
        return now + Pacific_tzinfo().utcoffset(now)

    @classmethod
    def get_service(cls, Auth):
        creds = Auth.get()
        if not creds:
            raise Exception('Credentials not found.')

        http = creds.authorize(httplib2.Http())
        creds.refresh(http)
        service = build('calendar', 'v3', credentials=creds)
        return service

class Pacific_tzinfo(datetime.tzinfo):
    """Implementation of the Pacific timezone."""
    def utcoffset(self, dt):
        return datetime.timedelta(hours=-8) + self.dst(dt)

    def _FirstSunday(self, dt):
        """First Sunday on or after dt."""
        return dt + datetime.timedelta(days=(6-dt.weekday()))

    def dst(self, dt):
        # 2 am on the second Sunday in March
        dst_start = self._FirstSunday(datetime.datetime(dt.year, 3, 8, 2))
        # 1 am on the first Sunday in November
        dst_end = self._FirstSunday(datetime.datetime(dt.year, 11, 1, 1))

        if dst_start <= dt.replace(tzinfo=None) < dst_end:
            return datetime.timedelta(hours=1)
        else:
            return datetime.timedelta(hours=0)

    def tzname(self, dt):
        if self.dst(dt) == datetime.timedelta(hours=0):
            return "PST"
        else:
            return "PDT"

# vim: set ts=4 sts=4 sw=4 et:
